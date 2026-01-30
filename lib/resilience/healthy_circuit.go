package resilience

import (
	"context"
	"net"
	"sync"
	"time"
)

// HealthyCircuitConfig configures the integrated health monitor with circuit breaker.
type HealthyCircuitConfig struct {
	// CircuitBreaker configuration
	CircuitBreaker CircuitBreakerConfig

	// Health check configuration
	CheckInterval time.Duration
	ProbeTimeout  time.Duration
}

// DefaultHealthyCircuitConfig returns sensible defaults.
func DefaultHealthyCircuitConfig() HealthyCircuitConfig {
	return HealthyCircuitConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(),
		CheckInterval:  30 * time.Second,
		ProbeTimeout:   5 * time.Second,
	}
}

// HealthyCircuit integrates health monitoring with circuit breaker pattern.
// It periodically checks the SAM bridge and uses results to drive circuit state.
type HealthyCircuit struct {
	mu     sync.RWMutex
	config HealthyCircuitConfig

	// SAM address to monitor
	samAddr string

	// Circuit breaker
	circuit *CircuitBreaker

	// Health check state
	lastCheck   time.Time
	lastHealthy time.Time
	isHealthy   bool

	// Callbacks
	onUnhealthy func()
	onHealthy   func()
	onReconnect func() error

	// Control
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewHealthyCircuit creates a new health monitor with integrated circuit breaker.
func NewHealthyCircuit(name, samAddr string, cfg HealthyCircuitConfig) *HealthyCircuit {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = DefaultHealthyCircuitConfig().CheckInterval
	}
	if cfg.ProbeTimeout == 0 {
		cfg.ProbeTimeout = DefaultHealthyCircuitConfig().ProbeTimeout
	}

	hc := &HealthyCircuit{
		config:    cfg,
		samAddr:   samAddr,
		circuit:   NewCircuitBreaker(name+"-circuit", cfg.CircuitBreaker),
		isHealthy: true, // Optimistic start
	}

	// Set up circuit state change callback for logging and metrics
	hc.circuit.SetStateChangeCallback(func(from, to CircuitState) {
		log.WithField("name", name).
			WithField("from", from.String()).
			WithField("to", to.String()).
			Info("circuit state changed")
		MetricsCallback(from, to)
	})

	return hc
}

// SetCallbacks sets the callbacks for health state changes.
func (hc *HealthyCircuit) SetCallbacks(onUnhealthy, onHealthy func(), onReconnect func() error) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onUnhealthy = onUnhealthy
	hc.onHealthy = onHealthy
	hc.onReconnect = onReconnect
}

// Start begins health monitoring.
func (hc *HealthyCircuit) Start(ctx context.Context) error {
	hc.mu.Lock()
	if hc.running {
		hc.mu.Unlock()
		return nil
	}
	hc.running = true
	ctx, cancel := context.WithCancel(ctx)
	hc.cancel = cancel
	hc.mu.Unlock()

	log.WithField("samAddr", hc.samAddr).
		WithField("checkInterval", hc.config.CheckInterval).
		Debug("starting health circuit monitor")

	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		hc.monitorLoop(ctx)
	}()

	return nil
}

// Stop halts health monitoring.
func (hc *HealthyCircuit) Stop() {
	hc.mu.Lock()
	if !hc.running {
		hc.mu.Unlock()
		return
	}
	hc.running = false
	hc.cancel()
	hc.mu.Unlock()

	hc.wg.Wait()
	log.Debug("health circuit monitor stopped")
}

// monitorLoop periodically checks SAM connectivity.
func (hc *HealthyCircuit) monitorLoop(ctx context.Context) {
	// Initial check
	hc.checkHealth()

	ticker := time.NewTicker(hc.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkHealth()
		}
	}
}

// checkHealth performs a health check and updates circuit breaker state.
func (hc *HealthyCircuit) checkHealth() {
	hc.mu.Lock()
	hc.lastCheck = time.Now()
	wasHealthy := hc.isHealthy
	onUnhealthy := hc.onUnhealthy
	onHealthy := hc.onHealthy
	onReconnect := hc.onReconnect
	hc.mu.Unlock()

	healthy := hc.probeSAM()

	hc.mu.Lock()
	hc.isHealthy = healthy
	if healthy {
		hc.lastHealthy = time.Now()
	}
	hc.mu.Unlock()

	if healthy {
		hc.circuit.RecordSuccess()
		if !wasHealthy && onHealthy != nil {
			log.Debug("SAM connection restored, invoking onHealthy callback")
			go onHealthy()
		}
	} else {
		hc.circuit.RecordFailure()
		if wasHealthy && onUnhealthy != nil {
			log.Debug("SAM connection unhealthy, invoking onUnhealthy callback")
			go onUnhealthy()
		}

		// Attempt reconnection if circuit is open or half-open
		if hc.circuit.IsOpen() && onReconnect != nil {
			log.Debug("circuit open, attempting reconnection")
			go func() {
				if err := onReconnect(); err != nil {
					log.WithError(err).Warn("reconnection attempt failed")
				}
			}()
		}
	}
}

// probeSAM attempts to connect to the SAM bridge.
func (hc *HealthyCircuit) probeSAM() bool {
	conn, err := net.DialTimeout("tcp", hc.samAddr, hc.config.ProbeTimeout)
	if err != nil {
		log.WithError(err).Debug("SAM probe failed")
		return false
	}
	conn.Close()
	return true
}

// Execute runs the given function if the circuit allows it.
// Use this to wrap I2P operations with circuit breaker protection.
func (hc *HealthyCircuit) Execute(fn func() error) error {
	return hc.circuit.Execute(fn)
}

// ExecuteWithContext runs the given function with context awareness.
func (hc *HealthyCircuit) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	return hc.circuit.ExecuteWithContext(ctx, fn)
}

// Allow checks if operations should be allowed based on circuit state.
func (hc *HealthyCircuit) Allow() bool {
	return hc.circuit.Allow()
}

// CircuitState returns the current circuit breaker state.
func (hc *HealthyCircuit) CircuitState() CircuitState {
	return hc.circuit.State()
}

// IsHealthy returns true if the last health check passed.
func (hc *HealthyCircuit) IsHealthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.isHealthy
}

// LastCheck returns the time of the last health check.
func (hc *HealthyCircuit) LastCheck() time.Time {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastCheck
}

// LastHealthy returns when the connection was last healthy.
func (hc *HealthyCircuit) LastHealthy() time.Time {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastHealthy
}

// Stats returns combined health and circuit breaker statistics.
func (hc *HealthyCircuit) Stats() HealthyCircuitStats {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	return HealthyCircuitStats{
		IsHealthy:      hc.isHealthy,
		LastCheck:      hc.lastCheck,
		LastHealthy:    hc.lastHealthy,
		CircuitBreaker: hc.circuit.Stats(),
	}
}

// HealthyCircuitStats holds combined statistics.
type HealthyCircuitStats struct {
	IsHealthy      bool
	LastCheck      time.Time
	LastHealthy    time.Time
	CircuitBreaker CircuitBreakerStats
}

// ForceCheck triggers an immediate health check.
func (hc *HealthyCircuit) ForceCheck() {
	hc.checkHealth()
}

// Reset resets both the circuit breaker and health state.
func (hc *HealthyCircuit) Reset() {
	hc.mu.Lock()
	hc.isHealthy = true
	hc.mu.Unlock()
	hc.circuit.Reset()
}
