package embedded

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestVPN_MonitorRaceCondition tests that the monitor goroutine safely handles
// concurrent access to v.node and v.ctx during shutdown sequences.
// This test attempts to trigger the race condition that existed before fixing
// the monitor() function to safely copy node/ctx references under mutex.
func TestVPN_MonitorRaceCondition(t *testing.T) {
	// Run the test multiple times to increase chance of detecting races
	for iteration := 0; iteration < 10; iteration++ {
		t.Run("iteration", func(t *testing.T) {
			vpn, err := New(Config{
				DataDir: t.TempDir(),
			})
			if err != nil {
				t.Fatalf("New failed: %v", err)
			}

			// Start and stop rapidly to try to trigger race between monitor accessing
			// v.node/v.ctx and Stop() clearing them
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			// Start VPN
			if err := vpn.Start(ctx); err != nil {
				t.Fatalf("Start failed: %v", err)
			}

			// Immediately close in a tight loop to maximize race window
			// The old code had a race where monitor could read v.node without mutex,
			// then Stop() could set it to nil, then monitor would call v.node.Done()
			if err := vpn.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		})
	}
}

// TestVPN_ConcurrentStartStopMonitor tests that concurrent Start/Stop operations
// don't cause panics in the monitor goroutine due to nil pointer dereferences.
func TestVPN_ConcurrentStartStopMonitor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race detection test in short mode")
	}

	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Start the VPN
	if err := vpn.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Create multiple goroutines that try to read state while stopping
	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Stop the VPN
	go func() {
		defer wg.Done()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := vpn.Stop(stopCtx); err != nil {
			t.Logf("Stop returned error (may be expected): %v", err)
		}
	}()

	// Goroutine 2: Read status repeatedly during shutdown
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = vpn.Status()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Goroutine 3: Check state repeatedly during shutdown
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = vpn.State()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Wait for all goroutines to complete
	wg.Wait()

	// If we get here without panicking, the race fix is working
	t.Log("No race detected - monitor safely handled concurrent access")
}

// TestVPN_MonitorNilNodeHandling verifies that monitor() safely handles
// the case where v.node is nil, which could happen during initialization failures.
func TestVPN_MonitorNilNodeHandling(t *testing.T) {
	// This test verifies the nil check still works correctly with the mutex protection
	vpn, err := New(Config{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer vpn.Close()

	// Don't start the VPN - node should remain nil
	// Call monitor directly (normally called by Start)
	// This should return immediately without panic
	done := make(chan struct{})
	go func() {
		vpn.monitor()
		close(done)
	}()

	// Monitor should exit quickly since node is nil
	select {
	case <-done:
		// Success - monitor returned without panicking
		t.Log("monitor() safely handled nil node")
	case <-time.After(2 * time.Second):
		t.Fatal("monitor() did not return within timeout when node is nil")
	}
}
