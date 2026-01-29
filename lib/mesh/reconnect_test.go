package mesh

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultReconnectConfig(t *testing.T) {
	cfg := DefaultReconnectConfig()

	if cfg.InitialDelay != 5*time.Second {
		t.Errorf("expected InitialDelay 5s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 5*time.Minute {
		t.Errorf("expected MaxDelay 5m, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %v", cfg.Multiplier)
	}
	if cfg.JitterFraction != 0.2 {
		t.Errorf("expected JitterFraction 0.2, got %v", cfg.JitterFraction)
	}
	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("expected CheckInterval 10s, got %v", cfg.CheckInterval)
	}
}

func TestReconnectManager_AddAndRemove(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay: time.Second,
	})

	// Initially empty
	if rm.QueuedCount() != 0 {
		t.Errorf("expected 0 queued, got %d", rm.QueuedCount())
	}

	// Add a peer
	rm.Add("node1", "dest1", []byte("token1"))
	if rm.QueuedCount() != 1 {
		t.Errorf("expected 1 queued, got %d", rm.QueuedCount())
	}
	if !rm.IsQueued("node1") {
		t.Error("expected node1 to be queued")
	}

	// Add same peer again (should be no-op)
	rm.Add("node1", "dest1-updated", []byte("token2"))
	if rm.QueuedCount() != 1 {
		t.Errorf("expected still 1 queued, got %d", rm.QueuedCount())
	}

	// Add another peer
	rm.Add("node2", "dest2", []byte("token2"))
	if rm.QueuedCount() != 2 {
		t.Errorf("expected 2 queued, got %d", rm.QueuedCount())
	}

	// Remove first peer
	rm.Remove("node1")
	if rm.QueuedCount() != 1 {
		t.Errorf("expected 1 queued, got %d", rm.QueuedCount())
	}
	if rm.IsQueued("node1") {
		t.Error("expected node1 to not be queued")
	}

	// Clear all
	rm.Clear()
	if rm.QueuedCount() != 0 {
		t.Errorf("expected 0 queued, got %d", rm.QueuedCount())
	}
}

func TestReconnectManager_QueuedPeers(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay: time.Second,
	})

	rm.Add("node1", "dest1", []byte("token1"))
	rm.Add("node2", "dest2", []byte("token2"))

	peers := rm.QueuedPeers()
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}

	// Check that the peers are in the list
	found := make(map[string]bool)
	for _, p := range peers {
		found[p.NodeID] = true
	}
	if !found["node1"] || !found["node2"] {
		t.Error("missing expected peers")
	}
}

func TestReconnectManager_CalculateBackoff(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:   time.Second,
		MaxDelay:       time.Minute,
		Multiplier:     2.0,
		JitterFraction: 0, // No jitter for deterministic tests
	})

	// First attempt: 1s
	delay := rm.calculateBackoff(0)
	if delay != time.Second {
		t.Errorf("expected 1s delay, got %v", delay)
	}

	// Second attempt: 2s
	delay = rm.calculateBackoff(1)
	if delay != 2*time.Second {
		t.Errorf("expected 2s delay, got %v", delay)
	}

	// Third attempt: 4s
	delay = rm.calculateBackoff(2)
	if delay != 4*time.Second {
		t.Errorf("expected 4s delay, got %v", delay)
	}

	// Fourth attempt: 8s
	delay = rm.calculateBackoff(3)
	if delay != 8*time.Second {
		t.Errorf("expected 8s delay, got %v", delay)
	}

	// After many attempts, should be capped at max
	delay = rm.calculateBackoff(10)
	if delay != time.Minute {
		t.Errorf("expected 60s (capped), got %v", delay)
	}
}

func TestReconnectManager_BackoffWithJitter(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:   10 * time.Second,
		MaxDelay:       time.Minute,
		Multiplier:     2.0,
		JitterFraction: 0.5, // Large jitter for testing
	})

	// Run multiple times to check jitter varies
	delays := make(map[time.Duration]bool)
	for i := 0; i < 10; i++ {
		delay := rm.calculateBackoff(0)
		delays[delay] = true

		// Should be between 5s and 15s (10s ± 50%)
		if delay < 5*time.Second || delay > 15*time.Second {
			t.Errorf("delay %v outside expected range [5s, 15s]", delay)
		}
	}

	// With 10 samples and 50% jitter, we should see some variation
	if len(delays) < 2 {
		t.Log("warning: jitter didn't produce variation (might be bad luck)")
	}
}

func TestReconnectManager_StartAndStop(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:  100 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Starting again should be a no-op
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("second start failed: %v", err)
	}

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	rm.Stop()

	// Stopping again should be a no-op
	rm.Stop()
}

func TestReconnectManager_ReconnectSuccess(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:  50 * time.Millisecond,
		CheckInterval: 20 * time.Millisecond,
	})

	var reconnectCalls int32

	// Handler that succeeds immediately
	rm.SetReconnectHandler(func(nodeID, i2pDest string, authToken []byte) error {
		atomic.AddInt32(&reconnectCalls, 1)
		return nil
	})

	rm.Add("node1", "dest1", []byte("token"))

	ctx := context.Background()
	rm.Start(ctx)
	defer rm.Stop()

	// Wait for reconnection to happen
	time.Sleep(150 * time.Millisecond)

	// Should have attempted reconnection and succeeded
	calls := atomic.LoadInt32(&reconnectCalls)
	if calls == 0 {
		t.Error("expected at least 1 reconnection attempt")
	}

	// Peer should be removed from queue after success
	if rm.IsQueued("node1") {
		t.Error("expected node1 to be removed from queue after successful reconnection")
	}
}

func TestReconnectManager_ReconnectFailureWithBackoff(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:   50 * time.Millisecond,
		MaxDelay:       200 * time.Millisecond,
		Multiplier:     2.0,
		JitterFraction: 0,
		CheckInterval:  20 * time.Millisecond,
	})

	var mu sync.Mutex
	var attempts []time.Time

	// Handler that always fails
	rm.SetReconnectHandler(func(nodeID, i2pDest string, authToken []byte) error {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()
		return errors.New("connection failed")
	})

	rm.Add("node1", "dest1", []byte("token"))

	ctx := context.Background()
	rm.Start(ctx)

	// Wait for multiple retry attempts
	time.Sleep(400 * time.Millisecond)

	rm.Stop()

	mu.Lock()
	numAttempts := len(attempts)
	mu.Unlock()

	// Should have at least 2 attempts
	if numAttempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", numAttempts)
	}

	// Check that the peer is still queued with error info
	peers := rm.QueuedPeers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer still queued, got %d", len(peers))
	}
	if peers[0].LastError != "connection failed" {
		t.Errorf("expected last error 'connection failed', got '%s'", peers[0].LastError)
	}
	if peers[0].Attempts < 2 {
		t.Errorf("expected at least 2 attempts recorded, got %d", peers[0].Attempts)
	}
}

func TestReconnectManager_MaxRetries(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:   20 * time.Millisecond,
		MaxDelay:       100 * time.Millisecond,
		Multiplier:     1.5,
		JitterFraction: 0,
		CheckInterval:  10 * time.Millisecond,
		MaxRetries:     3,
	})

	var attempts int32

	// Handler that always fails
	rm.SetReconnectHandler(func(nodeID, i2pDest string, authToken []byte) error {
		atomic.AddInt32(&attempts, 1)
		return errors.New("permanent failure")
	})

	rm.Add("node1", "dest1", []byte("token"))

	ctx := context.Background()
	rm.Start(ctx)

	// Wait for retries to exhaust
	time.Sleep(300 * time.Millisecond)

	rm.Stop()

	// Should have exactly MaxRetries attempts
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected exactly 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}

	// Peer should be removed after max retries
	if rm.IsQueued("node1") {
		t.Error("expected node1 to be removed after max retries")
	}
}

func TestReconnectManager_RemoveDuringReconnect(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:  10 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
	})

	reconnecting := make(chan struct{})
	done := make(chan struct{})

	// Handler that blocks until signaled
	rm.SetReconnectHandler(func(nodeID, i2pDest string, authToken []byte) error {
		close(reconnecting)
		<-done
		return errors.New("cancelled")
	})

	rm.Add("node1", "dest1", []byte("token"))

	ctx := context.Background()
	rm.Start(ctx)

	// Wait for reconnection to start
	<-reconnecting

	// Remove the peer while reconnection is in progress
	rm.Remove("node1")

	// Let the handler complete
	close(done)

	time.Sleep(50 * time.Millisecond)
	rm.Stop()

	// Peer should stay removed
	if rm.IsQueued("node1") {
		t.Error("expected node1 to stay removed")
	}
}

func TestReconnectManager_NoHandler(t *testing.T) {
	rm := NewReconnectManager(ReconnectConfig{
		InitialDelay:  10 * time.Millisecond,
		CheckInterval: 10 * time.Millisecond,
	})

	rm.Add("node1", "dest1", []byte("token"))

	ctx := context.Background()
	rm.Start(ctx)

	// Should not panic without a handler
	time.Sleep(50 * time.Millisecond)

	rm.Stop()

	// Peer should still be queued (handler not set = nothing happens)
	if !rm.IsQueued("node1") {
		t.Error("expected node1 to still be queued")
	}
}
