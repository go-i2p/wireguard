// Package transport provides I2P router health monitoring and automatic reconnection.
package transport

import (
	"context"
	"net"
	"sync"
	"time"
)

// HealthConfig configures the health monitor.
type HealthConfig struct {
	// CheckInterval is how often to check SAM connectivity.
	CheckInterval time.Duration
	// Timeout is the timeout for health check attempts.
	Timeout time.Duration
	// RetryDelay is how long to wait before attempting reconnection.
	RetryDelay time.Duration
	// MaxRetryDelay is the maximum retry delay.
	MaxRetryDelay time.Duration
}

// DefaultHealthConfig returns sensible defaults.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		CheckInterval: 30 * time.Second,
		Timeout:       5 * time.Second,
		RetryDelay:    5 * time.Second,
		MaxRetryDelay: 2 * time.Minute,
	}
}

// HealthState represents the health state of the I2P connection.
type HealthState int

const (
	// HealthStateUnknown is the initial state.
	HealthStateUnknown HealthState = iota
	// HealthStateHealthy means SAM is connected and responding.
	HealthStateHealthy
	// HealthStateUnhealthy means SAM is not responding.
	HealthStateUnhealthy
	// HealthStateReconnecting means we're attempting to reconnect.
	HealthStateReconnecting
)

func (s HealthState) String() string {
	switch s {
	case HealthStateUnknown:
		return "unknown"
	case HealthStateHealthy:
		return "healthy"
	case HealthStateUnhealthy:
		return "unhealthy"
	case HealthStateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// HealthMonitor monitors the health of the I2P SAM connection.
type HealthMonitor struct {
	mu     sync.RWMutex
	config HealthConfig

	// SAM address to monitor
	samAddr string

	// Current state
	state            HealthState
	lastCheck        time.Time
	lastHealthy      time.Time
	consecutiveFails int

	// Callbacks
	onUnhealthy func()
	onHealthy   func()
	onReconnect func() error

	// Control
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(samAddr string, cfg HealthConfig) *HealthMonitor {
	if cfg.CheckInterval == 0 {
		cfg = DefaultHealthConfig()
	}

	return &HealthMonitor{
		config:  cfg,
		samAddr: samAddr,
		state:   HealthStateUnknown,
	}
}

// SetCallbacks sets the callbacks for health state changes.
// onUnhealthy is called when SAM becomes unreachable.
// onHealthy is called when SAM becomes reachable after being unhealthy.
// onReconnect is called when reconnection is needed and should return error if it fails.
func (hm *HealthMonitor) SetCallbacks(onUnhealthy, onHealthy func(), onReconnect func() error) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.onUnhealthy = onUnhealthy
	hm.onHealthy = onHealthy
	hm.onReconnect = onReconnect
}

// Start begins health monitoring.
func (hm *HealthMonitor) Start(ctx context.Context) error {
	hm.mu.Lock()
	if hm.running {
		hm.mu.Unlock()
		return nil
	}
	hm.running = true

	ctx, cancel := context.WithCancel(ctx)
	hm.cancel = cancel
	hm.mu.Unlock()

	log.WithField("samAddr", hm.samAddr).WithField("checkInterval", hm.config.CheckInterval).Debug("starting I2P health monitor")

	hm.wg.Add(1)
	go func() {
		defer hm.wg.Done()
		hm.monitorLoop(ctx)
	}()

	return nil
}

// Stop halts health monitoring.
func (hm *HealthMonitor) Stop() {
	hm.mu.Lock()
	if !hm.running {
		hm.mu.Unlock()
		return
	}
	hm.running = false
	hm.cancel()
	hm.mu.Unlock()

	hm.wg.Wait()
	log.Debug("health monitor stopped")
}

// State returns the current health state.
func (hm *HealthMonitor) State() HealthState {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.state
}

// IsHealthy returns true if the connection is healthy.
func (hm *HealthMonitor) IsHealthy() bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.state == HealthStateHealthy
}

// LastCheck returns the time of the last health check.
func (hm *HealthMonitor) LastCheck() time.Time {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.lastCheck
}

// LastHealthy returns the time when the connection was last healthy.
func (hm *HealthMonitor) LastHealthy() time.Time {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.lastHealthy
}

// ConsecutiveFailures returns the number of consecutive check failures.
func (hm *HealthMonitor) ConsecutiveFailures() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.consecutiveFails
}

// monitorLoop periodically checks SAM connectivity.
func (hm *HealthMonitor) monitorLoop(ctx context.Context) {
	// Initial check
	hm.checkHealth()

	ticker := time.NewTicker(hm.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.checkHealth()
		}
	}
}

// checkHealth performs a single health check.
func (hm *HealthMonitor) checkHealth() {
	hm.mu.Lock()
	hm.lastCheck = time.Now()
	previousState := hm.state
	hm.mu.Unlock()

	healthy := hm.probeSAM()

	hm.mu.Lock()
	defer hm.mu.Unlock()

	if healthy {
		hm.handleHealthyState(previousState)
	} else {
		hm.handleUnhealthyState()
	}
}

// handleHealthyState processes a healthy SAM connection check result.
func (hm *HealthMonitor) handleHealthyState(previousState HealthState) {
	hm.consecutiveFails = 0
	hm.lastHealthy = time.Now()

	if hm.state == HealthStateHealthy {
		return
	}

	hm.state = HealthStateHealthy
	log.Debug("I2P SAM connection healthy")

	if (previousState == HealthStateUnhealthy || previousState == HealthStateReconnecting) && hm.onHealthy != nil {
		go hm.onHealthy()
	}
}

// handleUnhealthyState processes an unhealthy SAM connection check result.
func (hm *HealthMonitor) handleUnhealthyState() {
	hm.consecutiveFails++

	if hm.state == HealthStateHealthy {
		hm.state = HealthStateUnhealthy
		log.WithField("consecutiveFails", hm.consecutiveFails).Warn("I2P SAM connection unhealthy")

		if hm.onUnhealthy != nil {
			go hm.onUnhealthy()
		}
	}

	if hm.consecutiveFails >= 2 && hm.onReconnect != nil {
		hm.state = HealthStateReconnecting
		hm.attemptReconnection()
	}
}

// attemptReconnection attempts to reconnect to the I2P SAM bridge.
func (hm *HealthMonitor) attemptReconnection() {
	reconnector := hm.onReconnect
	hm.mu.Unlock()

	log.WithField("consecutiveFails", hm.consecutiveFails).Debug("attempting to reconnect to I2P")

	if err := reconnector(); err != nil {
		log.WithError(err).Error("reconnection failed")
	}

	hm.mu.Lock()
}

// probeSAM attempts to connect to the SAM bridge to verify it's running.
func (hm *HealthMonitor) probeSAM() bool {
	conn, err := net.DialTimeout("tcp", hm.samAddr, hm.config.Timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ForceCheck triggers an immediate health check.
func (hm *HealthMonitor) ForceCheck() {
	hm.checkHealth()
}
