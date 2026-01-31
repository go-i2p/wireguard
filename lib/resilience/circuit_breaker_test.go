package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreakerDefaultConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	if cfg.FailureThreshold <= 0 {
		t.Error("FailureThreshold should be positive")
	}
	if cfg.SuccessThreshold <= 0 {
		t.Error("SuccessThreshold should be positive")
	}
	if cfg.Timeout <= 0 {
		t.Error("Timeout should be positive")
	}
	if cfg.MaxHalfOpenRequests <= 0 {
		t.Error("MaxHalfOpenRequests should be positive")
	}
}

func TestCircuitBreakerInitialState(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())
	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state Closed, got %v", cb.State())
	}
	if !cb.IsClosed() {
		t.Error("expected IsClosed to be true")
	}
	if cb.IsOpen() {
		t.Error("expected IsOpen to be false")
	}
	if cb.IsHalfOpen() {
		t.Error("expected IsHalfOpen to be false")
	}
}

func TestCircuitBreakerName(t *testing.T) {
	cb := NewCircuitBreaker("my-circuit", DefaultCircuitBreakerConfig())
	if cb.Name() != "my-circuit" {
		t.Errorf("expected name 'my-circuit', got '%s'", cb.Name())
	}
}

func TestCircuitBreakerAllowWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())
	if !cb.Allow() {
		t.Error("expected Allow to return true when closed")
	}
}

func TestCircuitBreakerOpensAfterFailures(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 2,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Record failures up to threshold
	for i := 0; i < cfg.FailureThreshold; i++ {
		if cb.State() == CircuitOpen {
			t.Errorf("circuit opened too early at failure %d", i)
		}
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be Open, got %v", cb.State())
	}
	if cb.Allow() {
		t.Error("expected Allow to return false when open")
	}
}

func TestCircuitBreakerRejectsWhenOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             10 * time.Second, // Long timeout
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be Open, got %v", cb.State())
	}

	// Should reject requests
	if cb.Allow() {
		t.Error("expected Allow to return false when open")
	}
}

func TestCircuitBreakerTransitionsToHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// State should show half-open after timeout
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected circuit to be HalfOpen after timeout, got %v", cb.State())
	}

	// Allow should transition and return true
	if !cb.Allow() {
		t.Error("expected Allow to return true in half-open state")
	}
}

func TestCircuitBreakerClosesAfterSuccessInHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 3,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Allow a request (transitions to half-open)
	cb.Allow()

	// Record successes
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected circuit to still be HalfOpen, got %v", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be Closed after successes, got %v", cb.State())
	}
}

func TestCircuitBreakerReopensOnFailureInHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 3,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Allow a request (transitions to half-open)
	cb.Allow()

	// Record one success, then failure
	cb.RecordSuccess()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be Open after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreakerLimitsHalfOpenRequests(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// First two requests should be allowed
	if !cb.Allow() {
		t.Error("expected first half-open request to be allowed")
	}
	if !cb.Allow() {
		t.Error("expected second half-open request to be allowed")
	}

	// Third should be rejected
	if cb.Allow() {
		t.Error("expected third half-open request to be rejected")
	}
}

func TestCircuitBreakerSuccessResetsFailureCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    1,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	// Record success - should reset failure count
	cb.RecordSuccess()

	// Another failure shouldn't open the circuit
	cb.RecordFailure()

	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be Closed, got %v", cb.State())
	}
}

func TestCircuitBreakerExecuteSuccess(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	err := cb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	stats := cb.Stats()
	if stats.FailureCount != 0 {
		t.Errorf("expected failure count 0, got %d", stats.FailureCount)
	}
}

func TestCircuitBreakerExecuteFailure(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)
	testErr := errors.New("test error")

	err := cb.Execute(func() error {
		return testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("expected test error, got %v", err)
	}

	stats := cb.Stats()
	if stats.FailureCount != 1 {
		t.Errorf("expected failure count 1, got %d", stats.FailureCount)
	}
}

func TestCircuitBreakerExecuteRejectsWhenOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    1,
		Timeout:             10 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()

	executed := false
	err := cb.Execute(func() error {
		executed = true
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if executed {
		t.Error("function should not have been executed")
	}
}

func TestCircuitBreakerExecuteWithContext(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())
	ctx := context.Background()

	err := cb.ExecuteWithContext(ctx, func(c context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCircuitBreakerExecuteWithContextCancelled(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cb.ExecuteWithContext(ctx, func(c context.Context) error {
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCircuitBreakerForceOpen(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	cb.ForceOpen()

	if cb.State() != CircuitOpen {
		t.Errorf("expected circuit to be Open, got %v", cb.State())
	}
}

func TestCircuitBreakerForceClose(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    1,
		Timeout:             10 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit
	cb.RecordFailure()

	// Force close
	cb.ForceClose()

	if cb.State() != CircuitClosed {
		t.Errorf("expected circuit to be Closed, got %v", cb.State())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    1,
		Timeout:             10 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	// Open the circuit and record some activity
	cb.RecordFailure()

	cb.Reset()

	stats := cb.Stats()
	if stats.State != CircuitClosed {
		t.Errorf("expected state Closed after reset, got %v", stats.State)
	}
	if stats.FailureCount != 0 {
		t.Errorf("expected failure count 0 after reset, got %d", stats.FailureCount)
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	cb := NewCircuitBreaker("test-stats", DefaultCircuitBreakerConfig())

	cb.RecordFailure()
	cb.RecordSuccess()

	stats := cb.Stats()
	if stats.Name != "test-stats" {
		t.Errorf("expected name 'test-stats', got '%s'", stats.Name)
	}
	if stats.State != CircuitClosed {
		t.Errorf("expected state Closed, got %v", stats.State)
	}
	if stats.FailureCount != 0 { // Reset by success
		t.Errorf("expected failure count 0, got %d", stats.FailureCount)
	}
}

func TestCircuitBreakerStateChangeCallback(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	cb := NewCircuitBreaker("test", cfg)

	var transitions []struct{ from, to CircuitState }
	var mu sync.Mutex

	cb.SetStateChangeCallback(func(from, to CircuitState) {
		mu.Lock()
		transitions = append(transitions, struct{ from, to CircuitState }{from, to})
		mu.Unlock()
	})

	// Trigger closed -> open
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for callback
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if len(transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(transitions))
	}
	if len(transitions) > 0 {
		if transitions[0].from != CircuitClosed || transitions[0].to != CircuitOpen {
			t.Errorf("unexpected transition: %v -> %v", transitions[0].from, transitions[0].to)
		}
	}
	mu.Unlock()
}

func TestCircuitBreakerConcurrency(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    100,
		SuccessThreshold:    10,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 50,
	}
	cb := NewCircuitBreaker("test", cfg)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cb.Allow()
				if n%2 == 0 {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
			}
		}(i)
	}
	wg.Wait()

	// Just verify no panic occurred and state is valid
	state := cb.State()
	if state != CircuitClosed && state != CircuitOpen && state != CircuitHalfOpen {
		t.Errorf("unexpected state: %v", state)
	}
}

func TestCircuitStateString(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreakerDefaultsApplied(t *testing.T) {
	// Test that zero-value config gets defaults applied
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{})

	stats := cb.Stats()
	if stats.Config.FailureThreshold <= 0 {
		t.Error("expected FailureThreshold to be set to default")
	}
	if stats.Config.SuccessThreshold <= 0 {
		t.Error("expected SuccessThreshold to be set to default")
	}
	if stats.Config.Timeout <= 0 {
		t.Error("expected Timeout to be set to default")
	}
	if stats.Config.MaxHalfOpenRequests <= 0 {
		t.Error("expected MaxHalfOpenRequests to be set to default")
	}
}

func TestMetricsCircuitBreaker(t *testing.T) {
	mcb := NewMetricsCircuitBreaker("test-metrics", DefaultCircuitBreakerConfig())

	// Execute successful operation
	err := mcb.Execute(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Execute failing operation
	testErr := errors.New("test failure")
	err = mcb.Execute(func() error {
		return testErr
	})
	if !errors.Is(err, testErr) {
		t.Errorf("expected test error, got %v", err)
	}
}
