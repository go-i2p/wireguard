// Package mesh provides reconnection logic with exponential backoff.
package mesh

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"
)

// ReconnectConfig configures the reconnection manager.
type ReconnectConfig struct {
	// InitialDelay is the first retry delay.
	InitialDelay time.Duration
	// MaxDelay is the maximum retry delay.
	MaxDelay time.Duration
	// Multiplier is the backoff multiplier (typically 2.0).
	Multiplier float64
	// MaxRetries is the maximum number of retries (0 = unlimited).
	MaxRetries int
	// JitterFraction is the random jitter factor (0.0-1.0).
	JitterFraction float64
	// CheckInterval is how often to check for peers to reconnect.
	CheckInterval time.Duration
	// Logger for reconnection operations.
	Logger *slog.Logger
}

// DefaultReconnectConfig returns sensible defaults.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		InitialDelay:   5 * time.Second,
		MaxDelay:       5 * time.Minute,
		Multiplier:     2.0,
		MaxRetries:     0, // unlimited
		JitterFraction: 0.2,
		CheckInterval:  10 * time.Second,
	}
}

// reconnectEntry tracks a peer that needs reconnection.
type reconnectEntry struct {
	NodeID    string
	I2PDest   string
	AuthToken []byte
	Attempts  int
	NextRetry time.Time
	LastError string
	CreatedAt time.Time
}

// ReconnectManager manages automatic peer reconnection with backoff.
type ReconnectManager struct {
	mu     sync.RWMutex
	config ReconnectConfig
	logger *slog.Logger

	// Peers queued for reconnection
	queue map[string]*reconnectEntry

	// Callback for performing actual reconnection
	doReconnect func(nodeID, i2pDest string, authToken []byte) error

	// Control
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewReconnectManager creates a new reconnection manager.
func NewReconnectManager(cfg ReconnectConfig) *ReconnectManager {
	if cfg.InitialDelay == 0 {
		cfg = DefaultReconnectConfig()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Multiplier == 0 {
		cfg.Multiplier = 2.0
	}

	return &ReconnectManager{
		config: cfg,
		logger: cfg.Logger,
		queue:  make(map[string]*reconnectEntry),
	}
}

// SetReconnectHandler sets the callback for performing reconnection attempts.
func (rm *ReconnectManager) SetReconnectHandler(fn func(nodeID, i2pDest string, authToken []byte) error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.doReconnect = fn
}

// Add queues a peer for reconnection.
func (rm *ReconnectManager) Add(nodeID, i2pDest string, authToken []byte) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Don't add if already queued
	if _, exists := rm.queue[nodeID]; exists {
		return
	}

	rm.queue[nodeID] = &reconnectEntry{
		NodeID:    nodeID,
		I2PDest:   i2pDest,
		AuthToken: authToken,
		Attempts:  0,
		NextRetry: time.Now().Add(rm.config.InitialDelay),
		CreatedAt: time.Now(),
	}

	rm.logger.Info("queued peer for reconnection",
		"node_id", nodeID,
		"next_retry", rm.config.InitialDelay)
}

// Remove removes a peer from the reconnection queue.
func (rm *ReconnectManager) Remove(nodeID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.queue, nodeID)
}

// IsQueued returns whether a peer is queued for reconnection.
func (rm *ReconnectManager) IsQueued(nodeID string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	_, exists := rm.queue[nodeID]
	return exists
}

// QueuedCount returns the number of peers queued for reconnection.
func (rm *ReconnectManager) QueuedCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.queue)
}

// QueuedPeers returns information about queued peers.
func (rm *ReconnectManager) QueuedPeers() []ReconnectInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]ReconnectInfo, 0, len(rm.queue))
	for _, entry := range rm.queue {
		result = append(result, ReconnectInfo{
			NodeID:    entry.NodeID,
			I2PDest:   entry.I2PDest,
			Attempts:  entry.Attempts,
			NextRetry: entry.NextRetry,
			LastError: entry.LastError,
		})
	}
	return result
}

// ReconnectInfo provides information about a queued peer.
type ReconnectInfo struct {
	NodeID    string
	I2PDest   string
	Attempts  int
	NextRetry time.Time
	LastError string
}

// Start begins the reconnection loop.
func (rm *ReconnectManager) Start(ctx context.Context) error {
	rm.mu.Lock()
	if rm.running {
		rm.mu.Unlock()
		return nil
	}
	rm.running = true

	ctx, cancel := context.WithCancel(ctx)
	rm.cancel = cancel
	rm.mu.Unlock()

	rm.logger.Info("starting reconnection manager",
		"check_interval", rm.config.CheckInterval,
		"initial_delay", rm.config.InitialDelay,
		"max_delay", rm.config.MaxDelay)

	rm.wg.Add(1)
	go func() {
		defer rm.wg.Done()
		rm.reconnectLoop(ctx)
	}()

	return nil
}

// Stop halts the reconnection manager.
func (rm *ReconnectManager) Stop() {
	rm.mu.Lock()
	if !rm.running {
		rm.mu.Unlock()
		return
	}
	rm.running = false
	rm.cancel()
	rm.mu.Unlock()

	rm.wg.Wait()
	rm.logger.Info("reconnection manager stopped")
}

// reconnectLoop periodically attempts to reconnect to queued peers.
func (rm *ReconnectManager) reconnectLoop(ctx context.Context) {
	ticker := time.NewTicker(rm.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.processQueue()
		}
	}
}

// processQueue attempts reconnection for all eligible peers.
func (rm *ReconnectManager) processQueue() {
	rm.mu.Lock()
	handler := rm.doReconnect
	rm.mu.Unlock()

	if handler == nil {
		return
	}

	// Get entries ready for retry
	now := time.Now()
	var toRetry []*reconnectEntry

	rm.mu.RLock()
	for _, entry := range rm.queue {
		if now.After(entry.NextRetry) {
			// Copy entry for processing outside lock
			entryCopy := *entry
			toRetry = append(toRetry, &entryCopy)
		}
	}
	rm.mu.RUnlock()

	// Attempt reconnections
	for _, entry := range toRetry {
		rm.attemptReconnect(entry, handler)
	}
}

// attemptReconnect tries to reconnect to a single peer.
func (rm *ReconnectManager) attemptReconnect(entry *reconnectEntry, handler func(nodeID, i2pDest string, authToken []byte) error) {
	rm.logger.Info("attempting reconnection",
		"node_id", entry.NodeID,
		"attempt", entry.Attempts+1)

	err := handler(entry.NodeID, entry.I2PDest, entry.AuthToken)

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Entry may have been removed while we were trying
	queuedEntry, exists := rm.queue[entry.NodeID]
	if !exists {
		return
	}

	if err == nil {
		// Success - remove from queue
		delete(rm.queue, entry.NodeID)
		rm.logger.Info("reconnection successful",
			"node_id", entry.NodeID,
			"attempts", entry.Attempts+1)
		return
	}

	// Failed - update entry with backoff
	queuedEntry.Attempts++
	queuedEntry.LastError = err.Error()

	// Check max retries
	if rm.config.MaxRetries > 0 && queuedEntry.Attempts >= rm.config.MaxRetries {
		delete(rm.queue, entry.NodeID)
		rm.logger.Warn("reconnection failed permanently",
			"node_id", entry.NodeID,
			"attempts", queuedEntry.Attempts,
			"error", err)
		return
	}

	// Calculate next retry with exponential backoff
	delay := rm.calculateBackoff(queuedEntry.Attempts)
	queuedEntry.NextRetry = time.Now().Add(delay)

	rm.logger.Info("reconnection failed, scheduling retry",
		"node_id", entry.NodeID,
		"attempt", queuedEntry.Attempts,
		"next_retry", delay,
		"error", err)
}

// calculateBackoff calculates the next backoff delay with jitter.
func (rm *ReconnectManager) calculateBackoff(attempts int) time.Duration {
	// Exponential backoff: initial * multiplier^attempts
	delay := float64(rm.config.InitialDelay) * math.Pow(rm.config.Multiplier, float64(attempts))

	// Cap at max delay (if set)
	if rm.config.MaxDelay > 0 && delay > float64(rm.config.MaxDelay) {
		delay = float64(rm.config.MaxDelay)
	}

	// Add jitter: ±jitterFraction
	if rm.config.JitterFraction > 0 {
		jitter := delay * rm.config.JitterFraction
		delay += (rand.Float64()*2 - 1) * jitter
	}

	// Ensure minimum of initial delay
	if delay < float64(rm.config.InitialDelay) {
		delay = float64(rm.config.InitialDelay)
	}

	return time.Duration(delay)
}

// Clear removes all peers from the reconnection queue.
func (rm *ReconnectManager) Clear() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.queue = make(map[string]*reconnectEntry)
}
