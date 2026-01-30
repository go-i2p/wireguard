// Package resilience provides resilience patterns for the i2plan mesh VPN.
// This file implements the circuit breaker pattern for I2P operations.
//
// The circuit breaker pattern prevents cascading failures by detecting when
// an external service (like the I2P SAM bridge) is unhealthy and temporarily
// stopping requests to allow recovery.
//
// State transitions:
//
//	Closed (normal) -> Open (failing) -> HalfOpen (testing) -> Closed
//	                     ^                    |
//	                     +--------------------+ (if test fails)
package resilience

import (
	"context"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int

const (
	// CircuitClosed is the normal operating state - requests pass through.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit is tripped - requests fail immediately.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the service recovered.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures the circuit breaker behavior.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes in half-open state
	// before closing the circuit.
	SuccessThreshold int
	// Timeout is the duration to wait before transitioning from open to half-open.
	Timeout time.Duration
	// MaxHalfOpenRequests is the maximum number of requests allowed in half-open state.
	MaxHalfOpenRequests int
}

// DefaultCircuitBreakerConfig returns sensible defaults for I2P operations.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu     sync.RWMutex
	config CircuitBreakerConfig
	name   string

	// Current state
	state CircuitState

	// Counters
	failureCount         int
	successCount         int
	halfOpenRequestCount int

	// Timestamps
	lastFailureTime time.Time
	lastStateChange time.Time
	openedAt        time.Time

	// Callbacks for state changes
	onStateChange func(from, to CircuitState)
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = DefaultCircuitBreakerConfig().FailureThreshold
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = DefaultCircuitBreakerConfig().SuccessThreshold
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultCircuitBreakerConfig().Timeout
	}
	if cfg.MaxHalfOpenRequests <= 0 {
		cfg.MaxHalfOpenRequests = DefaultCircuitBreakerConfig().MaxHalfOpenRequests
	}

	return &CircuitBreaker{
		config:          cfg,
		name:            name,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
	}
}

// SetStateChangeCallback sets the callback for state changes.
func (cb *CircuitBreaker) SetStateChangeCallback(fn func(from, to CircuitState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.stateWithTimeCheck()
}

// stateWithTimeCheck returns the current state, transitioning from open
// to half-open if the timeout has elapsed. Must be called with at least a read lock.
func (cb *CircuitBreaker) stateWithTimeCheck() CircuitState {
	if cb.state == CircuitOpen && time.Since(cb.openedAt) >= cb.config.Timeout {
		// We need to upgrade to a write lock to transition state
		// This is handled by Allow() and Execute() which check again
		return CircuitHalfOpen
	}
	return cb.state
}

// Allow checks if a request should be allowed.
// Returns true if the request can proceed, false if it should be rejected.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.allowLocked()
}

// allowLocked checks if a request should be allowed. Must be called with the lock held.
func (cb *CircuitBreaker) allowLocked() bool {
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should transition to half-open
		if time.Since(cb.openedAt) >= cb.config.Timeout {
			cb.transitionTo(CircuitHalfOpen)
			cb.halfOpenRequestCount = 1
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow limited requests in half-open state
		if cb.halfOpenRequestCount < cb.config.MaxHalfOpenRequests {
			cb.halfOpenRequestCount++
			return true
		}
		return false
	default:
		return false
	}
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		cb.failureCount = 0
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.transitionTo(CircuitClosed)
		}
	case CircuitOpen:
		// Shouldn't happen, but handle gracefully
		log.WithField("circuit", cb.name).Warn("success recorded while circuit open")
	}
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen)
		}
	case CircuitHalfOpen:
		// Failed while testing - go back to open
		cb.transitionTo(CircuitOpen)
	case CircuitOpen:
		// Already open, no state change needed
	}
}

// transitionTo changes the circuit state. Must be called with the lock held.
func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters based on new state
	switch newState {
	case CircuitClosed:
		cb.failureCount = 0
		cb.successCount = 0
	case CircuitOpen:
		cb.openedAt = time.Now()
		cb.successCount = 0
	case CircuitHalfOpen:
		cb.successCount = 0
		cb.halfOpenRequestCount = 0
	}

	log.WithField("circuit", cb.name).
		WithField("from", oldState.String()).
		WithField("to", newState.String()).
		Info("circuit breaker state transition")

	if cb.onStateChange != nil {
		// Call callback without lock to avoid deadlocks
		go cb.onStateChange(oldState, newState)
	}
}

// Execute runs the given function if the circuit allows it.
// Returns ErrCircuitOpen if the circuit is open and the request is rejected.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// ExecuteWithContext runs the given function with context awareness.
// Respects context cancellation and records the result appropriately.
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	// Check context before executing
	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := fn(ctx)
	if err != nil {
		// Don't count context cancellation as a failure
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// ForceOpen forces the circuit to open state (for testing or manual intervention).
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(CircuitOpen)
}

// ForceClose forces the circuit to closed state (for testing or manual intervention).
func (cb *CircuitBreaker) ForceClose() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(CircuitClosed)
}

// Reset resets the circuit breaker to its initial closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequestCount = 0
	cb.lastStateChange = time.Now()
	cb.openedAt = time.Time{}
}

// Stats returns current circuit breaker statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		Name:                 cb.name,
		State:                cb.stateWithTimeCheck(),
		FailureCount:         cb.failureCount,
		SuccessCount:         cb.successCount,
		LastFailureTime:      cb.lastFailureTime,
		LastStateChange:      cb.lastStateChange,
		HalfOpenRequestCount: cb.halfOpenRequestCount,
		Config:               cb.config,
	}
}

// CircuitBreakerStats holds statistics for a circuit breaker.
type CircuitBreakerStats struct {
	Name                 string
	State                CircuitState
	FailureCount         int
	SuccessCount         int
	LastFailureTime      time.Time
	LastStateChange      time.Time
	HalfOpenRequestCount int
	Config               CircuitBreakerConfig
}

// IsOpen returns true if the circuit is currently open (rejecting requests).
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == CircuitOpen
}

// IsClosed returns true if the circuit is currently closed (allowing requests).
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.State() == CircuitClosed
}

// IsHalfOpen returns true if the circuit is currently half-open (testing).
func (cb *CircuitBreaker) IsHalfOpen() bool {
	return cb.State() == CircuitHalfOpen
}

// Name returns the name of this circuit breaker.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}
