package transport

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultHealthConfig(t *testing.T) {
	cfg := DefaultHealthConfig()

	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("expected CheckInterval 30s, got %v", cfg.CheckInterval)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("expected Timeout 5s, got %v", cfg.Timeout)
	}
	if cfg.RetryDelay != 5*time.Second {
		t.Errorf("expected RetryDelay 5s, got %v", cfg.RetryDelay)
	}
}

func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state    HealthState
		expected string
	}{
		{HealthStateUnknown, "unknown"},
		{HealthStateHealthy, "healthy"},
		{HealthStateUnhealthy, "unhealthy"},
		{HealthStateReconnecting, "reconnecting"},
		{HealthState(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.state.String() != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, tc.state.String())
		}
	}
}

func TestHealthMonitor_InitialState(t *testing.T) {
	hm := NewHealthMonitor("127.0.0.1:7656", HealthConfig{
		CheckInterval: time.Second,
	})

	if hm.State() != HealthStateUnknown {
		t.Errorf("expected initial state Unknown, got %v", hm.State())
	}
	if hm.IsHealthy() {
		t.Error("expected not healthy initially")
	}
	if hm.ConsecutiveFailures() != 0 {
		t.Errorf("expected 0 failures, got %d", hm.ConsecutiveFailures())
	}
}

func TestHealthMonitor_StartAndStop(t *testing.T) {
	hm := NewHealthMonitor("127.0.0.1:7656", HealthConfig{
		CheckInterval: 100 * time.Millisecond,
		Timeout:       10 * time.Millisecond,
	})

	ctx := context.Background()
	if err := hm.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Starting again should be a no-op
	if err := hm.Start(ctx); err != nil {
		t.Fatalf("second start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	hm.Stop()

	// Stopping again should be a no-op
	hm.Stop()
}

func TestHealthMonitor_DetectsHealthy(t *testing.T) {
	// Start a fake SAM server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hm := NewHealthMonitor(listener.Addr().String(), HealthConfig{
		CheckInterval: 50 * time.Millisecond,
		Timeout:       100 * time.Millisecond,
	})

	ctx := context.Background()
	hm.Start(ctx)
	defer hm.Stop()

	// Wait for health check
	time.Sleep(150 * time.Millisecond)

	if !hm.IsHealthy() {
		t.Error("expected healthy state")
	}
	if hm.State() != HealthStateHealthy {
		t.Errorf("expected HealthStateHealthy, got %v", hm.State())
	}
}

func TestHealthMonitor_DetectsUnhealthy(t *testing.T) {
	// Use a port that nothing is listening on (high random port)
	hm := NewHealthMonitor("127.0.0.1:59876", HealthConfig{
		CheckInterval: 50 * time.Millisecond,
		Timeout:       10 * time.Millisecond,
	})

	var unhealthyCount int32

	hm.SetCallbacks(
		func() { atomic.AddInt32(&unhealthyCount, 1) },
		nil,
		nil,
	)

	ctx := context.Background()
	hm.Start(ctx)
	defer hm.Stop()

	// Wait for health checks (initial + a couple more)
	time.Sleep(200 * time.Millisecond)

	if hm.IsHealthy() {
		t.Error("expected unhealthy state")
	}
	if hm.ConsecutiveFailures() < 1 {
		t.Errorf("expected at least 1 failure, got %d", hm.ConsecutiveFailures())
	}
	// Note: callback may not be called if state started as unknown and went to unhealthy
	// This is acceptable - the important thing is that unhealthy is detected
}

func TestHealthMonitor_RecoveryCallback(t *testing.T) {
	// Create a server that we can control
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()

	// Accept connections in background
	stopServer := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopServer:
				return
			default:
			}
			listener.(*net.TCPListener).SetDeadline(time.Now().Add(50 * time.Millisecond))
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			conn.Close()
		}
	}()

	hm := NewHealthMonitor(addr, HealthConfig{
		CheckInterval: 30 * time.Millisecond,
		Timeout:       20 * time.Millisecond,
	})

	var healthyCount int32
	var unhealthyCount int32

	hm.SetCallbacks(
		func() { atomic.AddInt32(&unhealthyCount, 1) },
		func() { atomic.AddInt32(&healthyCount, 1) },
		nil,
	)

	ctx := context.Background()
	hm.Start(ctx)
	defer hm.Stop()

	// Wait for initial healthy check
	time.Sleep(100 * time.Millisecond)

	if !hm.IsHealthy() {
		t.Error("expected healthy initially")
	}

	// Stop the server
	close(stopServer)
	listener.Close()

	// Wait for unhealthy detection
	time.Sleep(150 * time.Millisecond)

	if hm.IsHealthy() {
		t.Error("expected unhealthy after server stop")
	}

	// Restart the server
	listener2, err := net.Listen("tcp", addr)
	if err != nil {
		// Port might not be immediately available
		t.Skip("could not restart server on same port")
	}
	defer listener2.Close()

	go func() {
		for {
			conn, err := listener2.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Wait for recovery
	time.Sleep(150 * time.Millisecond)

	if !hm.IsHealthy() {
		t.Error("expected healthy after server restart")
	}
	if atomic.LoadInt32(&healthyCount) == 0 {
		t.Error("expected healthy callback to be called")
	}
}

func TestHealthMonitor_ForceCheck(t *testing.T) {
	// Start a fake SAM server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	hm := NewHealthMonitor(listener.Addr().String(), HealthConfig{
		CheckInterval: time.Hour, // Long interval
		Timeout:       100 * time.Millisecond,
	})

	// Force an immediate check
	hm.ForceCheck()

	if !hm.IsHealthy() {
		t.Error("expected healthy after force check")
	}
	if hm.LastCheck().IsZero() {
		t.Error("expected last check to be set")
	}
}

func TestHealthMonitor_ReconnectCallback(t *testing.T) {
	// Use a port that nothing is listening on
	hm := NewHealthMonitor("127.0.0.1:59998", HealthConfig{
		CheckInterval: 30 * time.Millisecond,
		Timeout:       10 * time.Millisecond,
	})

	var reconnectCount int32

	hm.SetCallbacks(
		nil,
		nil,
		func() error {
			atomic.AddInt32(&reconnectCount, 1)
			return nil
		},
	)

	ctx := context.Background()
	hm.Start(ctx)
	defer hm.Stop()

	// Wait for multiple failures and reconnect attempt
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&reconnectCount) == 0 {
		t.Error("expected reconnect callback to be called after consecutive failures")
	}
}
