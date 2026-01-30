package resilience

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestHealthyCircuitDefaultConfig(t *testing.T) {
	cfg := DefaultHealthyCircuitConfig()
	if cfg.CheckInterval <= 0 {
		t.Error("CheckInterval should be positive")
	}
	if cfg.ProbeTimeout <= 0 {
		t.Error("ProbeTimeout should be positive")
	}
	if cfg.CircuitBreaker.FailureThreshold <= 0 {
		t.Error("CircuitBreaker.FailureThreshold should be positive")
	}
}

func TestHealthyCircuitInitialState(t *testing.T) {
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	if !hc.IsHealthy() {
		t.Error("expected initial state to be healthy")
	}
	if hc.CircuitState() != CircuitClosed {
		t.Errorf("expected initial circuit state Closed, got %v", hc.CircuitState())
	}
}

func TestHealthyCircuitStats(t *testing.T) {
	hc := NewHealthyCircuit("test-stats", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	stats := hc.Stats()
	if !stats.IsHealthy {
		t.Error("expected initial health to be true")
	}
	if stats.CircuitBreaker.State != CircuitClosed {
		t.Errorf("expected circuit state Closed, got %v", stats.CircuitBreaker.State)
	}
}

func TestHealthyCircuitAllow(t *testing.T) {
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	// Should allow initially
	if !hc.Allow() {
		t.Error("expected Allow to return true initially")
	}
}

func TestHealthyCircuitReset(t *testing.T) {
	cfg := HealthyCircuitConfig{
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:    1,
			SuccessThreshold:    1,
			Timeout:             1 * time.Second,
			MaxHalfOpenRequests: 1,
		},
		CheckInterval: 1 * time.Second,
		ProbeTimeout:  100 * time.Millisecond,
	}
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", cfg)
	defer hc.Stop()

	// Force open state via circuit breaker
	hc.circuit.ForceOpen()

	if hc.CircuitState() != CircuitOpen {
		t.Error("expected circuit to be open")
	}

	hc.Reset()

	if !hc.IsHealthy() {
		t.Error("expected IsHealthy to be true after reset")
	}
	if hc.CircuitState() != CircuitClosed {
		t.Error("expected circuit to be closed after reset")
	}
}

func TestHealthyCircuitExecute(t *testing.T) {
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	executed := false
	err := hc.Execute(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !executed {
		t.Error("expected function to be executed")
	}
}

func TestHealthyCircuitExecuteWithContext(t *testing.T) {
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	ctx := context.Background()
	executed := false
	err := hc.ExecuteWithContext(ctx, func(c context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !executed {
		t.Error("expected function to be executed")
	}
}

func TestHealthyCircuitSetCallbacks(t *testing.T) {
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", DefaultHealthyCircuitConfig())
	defer hc.Stop()

	unhealthyCalled := false
	healthyCalled := false
	reconnectCalled := false

	hc.SetCallbacks(
		func() { unhealthyCalled = true },
		func() { healthyCalled = true },
		func() error { reconnectCalled = true; return nil },
	)

	// Just verify callbacks are set without error
	_ = unhealthyCalled
	_ = healthyCalled
	_ = reconnectCalled
}

func TestHealthyCircuitStartStop(t *testing.T) {
	cfg := HealthyCircuitConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(),
		CheckInterval:  50 * time.Millisecond,
		ProbeTimeout:   10 * time.Millisecond,
	}
	hc := NewHealthyCircuit("test", "127.0.0.1:7656", cfg)

	ctx := context.Background()
	err := hc.Start(ctx)
	if err != nil {
		t.Errorf("expected no error starting, got %v", err)
	}

	// Double start should be ok
	err = hc.Start(ctx)
	if err != nil {
		t.Errorf("expected no error on double start, got %v", err)
	}

	hc.Stop()

	// Double stop should be ok
	hc.Stop()
}

func TestHealthyCircuitWithRealListener(t *testing.T) {
	// Start a real TCP listener to simulate SAM
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	cfg := HealthyCircuitConfig{
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:    2,
			SuccessThreshold:    1,
			Timeout:             100 * time.Millisecond,
			MaxHalfOpenRequests: 1,
		},
		CheckInterval: 50 * time.Millisecond,
		ProbeTimeout:  100 * time.Millisecond,
	}

	hc := NewHealthyCircuit("test", listener.Addr().String(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = hc.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer hc.Stop()

	// Wait for first health check
	time.Sleep(60 * time.Millisecond)

	// Should be healthy since listener is up
	if !hc.IsHealthy() {
		t.Error("expected healthy with listener up")
	}
	if hc.CircuitState() != CircuitClosed {
		t.Errorf("expected circuit closed, got %v", hc.CircuitState())
	}
}

func TestHealthyCircuitProbeFailure(t *testing.T) {
	cfg := HealthyCircuitConfig{
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:    2,
			SuccessThreshold:    1,
			Timeout:             100 * time.Millisecond,
			MaxHalfOpenRequests: 1,
		},
		CheckInterval: 50 * time.Millisecond,
		ProbeTimeout:  10 * time.Millisecond,
	}

	// Use an address that will fail
	hc := NewHealthyCircuit("test", "127.0.0.1:1", cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := hc.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer hc.Stop()

	// Wait for checks to fail
	time.Sleep(150 * time.Millisecond)

	// Should be unhealthy
	if hc.IsHealthy() {
		t.Error("expected unhealthy with no listener")
	}

	// Circuit should be open after failures
	if hc.CircuitState() != CircuitOpen {
		t.Errorf("expected circuit open, got %v", hc.CircuitState())
	}
}

func TestHealthyCircuitForceCheck(t *testing.T) {
	cfg := HealthyCircuitConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(),
		CheckInterval:  1 * time.Hour, // Long interval
		ProbeTimeout:   10 * time.Millisecond,
	}

	hc := NewHealthyCircuit("test", "127.0.0.1:1", cfg)
	defer hc.Stop()

	// Initial state
	if hc.LastCheck().IsZero() {
		// Force a check
		hc.ForceCheck()
	}

	lastCheck := hc.LastCheck()
	if lastCheck.IsZero() {
		t.Error("expected LastCheck to be set after ForceCheck")
	}
}

func TestHealthyCircuitLastHealthy(t *testing.T) {
	// Start a real TCP listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	cfg := HealthyCircuitConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(),
		CheckInterval:  1 * time.Hour,
		ProbeTimeout:   100 * time.Millisecond,
	}

	hc := NewHealthyCircuit("test", listener.Addr().String(), cfg)
	defer hc.Stop()

	// Force a check
	hc.ForceCheck()

	lastHealthy := hc.LastHealthy()
	if lastHealthy.IsZero() {
		t.Error("expected LastHealthy to be set after successful check")
	}
}
