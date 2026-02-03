// Package metrics provides monitoring and metrics collection for exit node functionality.
package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// ExitMetrics contains real-time metrics for an exit node.
// All fields are safe for concurrent access using atomic operations.
type ExitMetrics struct {
	// ActiveClients is the current number of connected clients
	activeClients atomic.Int64

	// BytesForwarded is the cumulative total bytes forwarded through exit
	bytesForwarded atomic.Uint64

	// BytesReceived is the cumulative total bytes received from clients
	bytesReceived atomic.Uint64

	// CurrentBandwidthMbps is the instantaneous bandwidth usage in megabits per second
	currentBandwidthMbps atomic.Uint64 // stored as uint64 to use atomic, divide by 1000 for float

	// DroppedPackets is the cumulative count of dropped packets
	droppedPackets atomic.Uint64

	// AverageLatencyNs is the average latency in nanoseconds
	averageLatencyNs atomic.Int64

	// StartTime records when metrics collection began
	startTime time.Time

	// LastUpdate records the last time metrics were updated
	lastUpdate atomic.Int64 // Unix timestamp in nanoseconds
}

// ExitMetricsCollector manages metrics collection for an exit node.
// It tracks connection statistics and calculates derived metrics like bandwidth and latency.
type ExitMetricsCollector struct {
	metrics         *ExitMetrics
	mu              sync.RWMutex
	connStats       map[string]*ConnectionStats // keyed by client ID
	bandwidthWindow time.Duration               // window for bandwidth calculation
	lastBwCalc      time.Time                   // last bandwidth calculation time
}

// ConnectionStats tracks per-connection statistics.
type ConnectionStats struct {
	ClientID       string
	BytesSent      uint64
	BytesReceived  uint64
	PacketsDropped uint64
	LatencySamples []time.Duration
	ConnectedAt    time.Time
	LastActivity   time.Time
}

// NewExitMetrics creates a new ExitMetrics instance.
func NewExitMetrics() *ExitMetrics {
	m := &ExitMetrics{
		startTime: time.Now(),
	}
	m.lastUpdate.Store(time.Now().UnixNano())
	return m
}

// NewExitMetricsCollector creates a new metrics collector with a specified bandwidth calculation window.
// A typical window is 10-30 seconds for responsive bandwidth monitoring.
func NewExitMetricsCollector(bandwidthWindow time.Duration) *ExitMetricsCollector {
	if bandwidthWindow <= 0 {
		bandwidthWindow = 10 * time.Second // default window
	}

	return &ExitMetricsCollector{
		metrics:         NewExitMetrics(),
		connStats:       make(map[string]*ConnectionStats),
		bandwidthWindow: bandwidthWindow,
		lastBwCalc:      time.Now(),
	}
}

// GetMetrics returns a snapshot of current metrics.
// The returned struct is a copy and safe to read without locking.
func (c *ExitMetricsCollector) GetMetrics() ExitMetrics {
	return ExitMetrics{
		startTime: c.metrics.startTime,
	}
}

// ActiveClients returns the current number of active clients.
func (m *ExitMetrics) ActiveClients() int64 {
	return m.activeClients.Load()
}

// BytesForwarded returns the total bytes forwarded.
func (m *ExitMetrics) BytesForwarded() uint64 {
	return m.bytesForwarded.Load()
}

// BytesReceived returns the total bytes received.
func (m *ExitMetrics) BytesReceived() uint64 {
	return m.bytesReceived.Load()
}

// CurrentBandwidthMbps returns the current bandwidth in Mbps as a float64.
func (m *ExitMetrics) CurrentBandwidthMbps() float64 {
	return float64(m.currentBandwidthMbps.Load()) / 1000.0
}

// DroppedPackets returns the total number of dropped packets.
func (m *ExitMetrics) DroppedPackets() uint64 {
	return m.droppedPackets.Load()
}

// AverageLatency returns the average latency as a time.Duration.
func (m *ExitMetrics) AverageLatency() time.Duration {
	return time.Duration(m.averageLatencyNs.Load())
}

// Uptime returns the duration since metrics collection started.
func (m *ExitMetrics) Uptime() time.Duration {
	return time.Since(m.startTime)
}

// LastUpdate returns the time of the last metrics update.
func (m *ExitMetrics) LastUpdate() time.Time {
	return time.Unix(0, m.lastUpdate.Load())
}

// Load returns the current load as a float between 0.0 and 1.0.
// Load is calculated based on active clients and bandwidth usage.
// Returns 0.0 if no load data is available.
func (m *ExitMetrics) Load() float64 {
	// Simple load calculation: bandwidth as percentage of 1Gbps + client factor
	// This can be adjusted based on actual hardware capabilities
	maxBandwidthMbps := 1000.0 // 1 Gbps
	bwLoad := m.CurrentBandwidthMbps() / maxBandwidthMbps

	// Client load: assume 100 clients = full load
	maxClients := 100.0
	clientLoad := float64(m.ActiveClients()) / maxClients

	// Weighted average: 70% bandwidth, 30% clients
	load := (bwLoad * 0.7) + (clientLoad * 0.3)

	if load > 1.0 {
		return 1.0
	}
	return load
}

// AddConnection registers a new client connection.
func (c *ExitMetricsCollector) AddConnection(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.connStats[clientID]; !exists {
		c.connStats[clientID] = &ConnectionStats{
			ClientID:       clientID,
			ConnectedAt:    time.Now(),
			LastActivity:   time.Now(),
			LatencySamples: make([]time.Duration, 0, 100), // preallocate
		}
		c.metrics.activeClients.Add(1)
	}
}

// RemoveConnection removes a client connection.
func (c *ExitMetricsCollector) RemoveConnection(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.connStats[clientID]; exists {
		delete(c.connStats, clientID)
		c.metrics.activeClients.Add(-1)
	}
}

// RecordBytesSent records bytes sent to a client.
func (c *ExitMetricsCollector) RecordBytesSent(clientID string, bytes uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if stats, exists := c.connStats[clientID]; exists {
		stats.BytesSent += bytes
		stats.LastActivity = time.Now()
	}
	c.metrics.bytesForwarded.Add(bytes)
	c.metrics.lastUpdate.Store(time.Now().UnixNano())
}

// RecordBytesReceived records bytes received from a client.
func (c *ExitMetricsCollector) RecordBytesReceived(clientID string, bytes uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if stats, exists := c.connStats[clientID]; exists {
		stats.BytesReceived += bytes
		stats.LastActivity = time.Now()
	}
	c.metrics.bytesReceived.Add(bytes)
	c.metrics.lastUpdate.Store(time.Now().UnixNano())
}

// RecordPacketDrop records a dropped packet for a client.
func (c *ExitMetricsCollector) RecordPacketDrop(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if stats, exists := c.connStats[clientID]; exists {
		stats.PacketsDropped++
	}
	c.metrics.droppedPackets.Add(1)
	c.metrics.lastUpdate.Store(time.Now().UnixNano())
}

// RecordLatency records a latency sample for a client.
// Only the most recent 100 samples per client are kept.
func (c *ExitMetricsCollector) RecordLatency(clientID string, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if stats, exists := c.connStats[clientID]; exists {
		// Keep only last 100 samples per client
		if len(stats.LatencySamples) >= 100 {
			stats.LatencySamples = stats.LatencySamples[1:]
		}
		stats.LatencySamples = append(stats.LatencySamples, latency)
	}

	// Calculate average latency across all clients
	c.updateAverageLatency()
	c.metrics.lastUpdate.Store(time.Now().UnixNano())
}

// updateAverageLatency calculates the average latency across all clients.
// Must be called with c.mu held.
func (c *ExitMetricsCollector) updateAverageLatency() {
	var totalLatency time.Duration
	var sampleCount int64

	for _, stats := range c.connStats {
		for _, sample := range stats.LatencySamples {
			totalLatency += sample
			sampleCount++
		}
	}

	if sampleCount > 0 {
		avgLatency := totalLatency / time.Duration(sampleCount)
		c.metrics.averageLatencyNs.Store(int64(avgLatency))
	}
}

// CalculateBandwidth calculates current bandwidth based on recent traffic.
// Should be called periodically (e.g., every 10 seconds) to update bandwidth metrics.
func (c *ExitMetricsCollector) CalculateBandwidth() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(c.lastBwCalc)
	if elapsed == 0 {
		return // avoid division by zero
	}

	// Calculate bytes transferred since last calculation
	var recentBytes uint64
	for _, stats := range c.connStats {
		recentBytes += stats.BytesSent + stats.BytesReceived
	}

	// Convert to Mbps: (bytes * 8) / (seconds * 1,000,000)
	// Store as milliMbps (Mbps * 1000) to maintain precision with uint64
	seconds := elapsed.Seconds()
	if seconds > 0 {
		mbps := (float64(recentBytes) * 8.0) / (seconds * 1_000_000.0)
		milliMbps := uint64(mbps * 1000.0)
		c.metrics.currentBandwidthMbps.Store(milliMbps)
	}

	c.lastBwCalc = now
}

// GetConnectionStats returns a copy of connection statistics.
func (c *ExitMetricsCollector) GetConnectionStats() map[string]ConnectionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]ConnectionStats, len(c.connStats))
	for k, v := range c.connStats {
		result[k] = *v
	}
	return result
}

// Reset resets all metrics to initial state.
func (c *ExitMetricsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = NewExitMetrics()
	c.connStats = make(map[string]*ConnectionStats)
	c.lastBwCalc = time.Now()
}
