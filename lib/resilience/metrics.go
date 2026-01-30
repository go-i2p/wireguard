package resilience

import (
	"github.com/go-i2p/wireguard/lib/metrics"
)

// Circuit breaker metrics for Prometheus exposition.
var (
	// CircuitBreakerState tracks the current state of each circuit breaker.
	// 0 = closed, 1 = open, 2 = half-open
	CircuitBreakerState = metrics.NewGauge(
		"i2plan_circuit_breaker_state",
		"Current state of the circuit breaker (0=closed, 1=open, 2=half-open)",
	)

	// CircuitBreakerTrips counts the number of times circuits have opened.
	CircuitBreakerTrips = metrics.NewCounter(
		"i2plan_circuit_breaker_trips_total",
		"Total number of times circuit breakers have opened",
	)

	// CircuitBreakerSuccesses counts successful operations through circuit breakers.
	CircuitBreakerSuccesses = metrics.NewCounter(
		"i2plan_circuit_breaker_successes_total",
		"Total successful operations through circuit breakers",
	)

	// CircuitBreakerFailures counts failed operations through circuit breakers.
	CircuitBreakerFailures = metrics.NewCounter(
		"i2plan_circuit_breaker_failures_total",
		"Total failed operations through circuit breakers",
	)

	// CircuitBreakerRejections counts requests rejected by open circuits.
	CircuitBreakerRejections = metrics.NewCounter(
		"i2plan_circuit_breaker_rejections_total",
		"Total requests rejected by open circuit breakers",
	)
)

// MetricsCallback creates a state change callback that updates metrics.
// Use this with SetStateChangeCallback to automatically track state transitions.
func MetricsCallback(from, to CircuitState) {
	CircuitBreakerState.Set(int64(to))
	if to == CircuitOpen {
		CircuitBreakerTrips.Inc()
	}
}

// MetricsCircuitBreaker wraps a CircuitBreaker with automatic metrics recording.
type MetricsCircuitBreaker struct {
	*CircuitBreaker
}

// NewMetricsCircuitBreaker creates a circuit breaker that automatically records metrics.
func NewMetricsCircuitBreaker(name string, cfg CircuitBreakerConfig) *MetricsCircuitBreaker {
	cb := NewCircuitBreaker(name, cfg)
	cb.SetStateChangeCallback(MetricsCallback)
	return &MetricsCircuitBreaker{CircuitBreaker: cb}
}

// Execute runs the function and records metrics for success/failure/rejection.
func (mcb *MetricsCircuitBreaker) Execute(fn func() error) error {
	if !mcb.Allow() {
		CircuitBreakerRejections.Inc()
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		CircuitBreakerFailures.Inc()
		mcb.RecordFailure()
		return err
	}

	CircuitBreakerSuccesses.Inc()
	mcb.RecordSuccess()
	return nil
}
