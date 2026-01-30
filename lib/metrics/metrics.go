// Package metrics provides simple metrics collection for i2plan.
// Supports Prometheus exposition format for monitoring integration.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Counter is a monotonically increasing counter.
type Counter struct {
	value uint64
	name  string
	help  string
}

// NewCounter creates a new counter metric.
func NewCounter(name, help string) *Counter {
	c := &Counter{
		name: name,
		help: help,
	}
	defaultRegistry.register(c)
	return c
}

// Inc increments the counter by 1.
func (c *Counter) Inc() {
	atomic.AddUint64(&c.value, 1)
}

// Add adds the given value to the counter.
func (c *Counter) Add(v uint64) {
	atomic.AddUint64(&c.value, v)
}

// Value returns the current counter value.
func (c *Counter) Value() uint64 {
	return atomic.LoadUint64(&c.value)
}

func (c *Counter) prometheus() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", c.name, c.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", c.name))
	sb.WriteString(fmt.Sprintf("%s %d\n", c.name, c.Value()))
	return sb.String()
}

// Gauge is a metric that can go up and down.
type Gauge struct {
	value int64
	name  string
	help  string
}

// NewGauge creates a new gauge metric.
func NewGauge(name, help string) *Gauge {
	g := &Gauge{
		name: name,
		help: help,
	}
	defaultRegistry.register(g)
	return g
}

// Set sets the gauge to the given value.
func (g *Gauge) Set(v int64) {
	atomic.StoreInt64(&g.value, v)
}

// Inc increments the gauge by 1.
func (g *Gauge) Inc() {
	atomic.AddInt64(&g.value, 1)
}

// Dec decrements the gauge by 1.
func (g *Gauge) Dec() {
	atomic.AddInt64(&g.value, -1)
}

// Add adds the given value to the gauge.
func (g *Gauge) Add(v int64) {
	atomic.AddInt64(&g.value, v)
}

// Value returns the current gauge value.
func (g *Gauge) Value() int64 {
	return atomic.LoadInt64(&g.value)
}

func (g *Gauge) prometheus() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", g.name, g.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", g.name))
	sb.WriteString(fmt.Sprintf("%s %d\n", g.name, g.Value()))
	return sb.String()
}

// Histogram tracks the distribution of values.
type Histogram struct {
	mu      sync.Mutex
	name    string
	help    string
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

// NewHistogram creates a new histogram metric.
func NewHistogram(name, help string, buckets []float64) *Histogram {
	h := &Histogram{
		name:    name,
		help:    help,
		buckets: buckets,
		counts:  make([]uint64, len(buckets)),
	}
	defaultRegistry.register(h)
	return h
}

// Observe records a value in the histogram.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += v
	h.count++

	for i, b := range h.buckets {
		if v <= b {
			h.counts[i]++
		}
	}
}

func (h *Histogram) prometheus() string {
	h.mu.Lock()
	defer h.mu.Unlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", h.name, h.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", h.name))

	for i, b := range h.buckets {
		sb.WriteString(fmt.Sprintf("%s_bucket{le=\"%g\"} %d\n", h.name, b, h.counts[i]))
	}
	sb.WriteString(fmt.Sprintf("%s_bucket{le=\"+Inf\"} %d\n", h.name, h.count))
	sb.WriteString(fmt.Sprintf("%s_sum %g\n", h.name, h.sum))
	sb.WriteString(fmt.Sprintf("%s_count %d\n", h.name, h.count))

	return sb.String()
}

// metric is the interface for all metric types.
type metric interface {
	prometheus() string
}

// Registry holds all registered metrics.
type Registry struct {
	mu      sync.RWMutex
	metrics map[string]metric
}

// defaultRegistry is the global metric registry.
var defaultRegistry = &Registry{
	metrics: make(map[string]metric),
}

func (r *Registry) register(m metric) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch v := m.(type) {
	case *Counter:
		r.metrics[v.name] = m
	case *Gauge:
		r.metrics[v.name] = m
	case *Histogram:
		r.metrics[v.name] = m
	}
}

// Expose returns all metrics in Prometheus exposition format.
func (r *Registry) Expose() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Sort names for consistent output
	names := make([]string, 0, len(r.metrics))
	for name := range r.metrics {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		sb.WriteString(r.metrics[name].prometheus())
		sb.WriteString("\n")
	}
	return sb.String()
}

// Handler returns an http.Handler that exposes metrics.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Write([]byte(defaultRegistry.Expose()))
	})
}

// Default metrics for i2plan
var (
	// Peer metrics
	PeersTotal     = NewGauge("i2plan_peers_total", "Total number of known peers")
	PeersConnected = NewGauge("i2plan_peers_connected", "Number of connected peers")

	// Message metrics
	MessagesReceived = NewCounter("i2plan_messages_received_total", "Total messages received")
	MessagesSent     = NewCounter("i2plan_messages_sent_total", "Total messages sent")

	// Gossip metrics
	GossipRoundsTotal = NewCounter("i2plan_gossip_rounds_total", "Total gossip rounds completed")
	AntiEntropyRounds = NewCounter("i2plan_antientropy_rounds_total", "Total anti-entropy rounds")

	// Gossip operation metrics (enhanced)
	GossipHeartbeatsSent   = NewCounter("i2plan_gossip_heartbeats_sent_total", "Total heartbeat announcements sent")
	GossipPeerListsSent    = NewCounter("i2plan_gossip_peer_lists_sent_total", "Total peer lists gossiped")
	GossipRouteUpdatesSent = NewCounter("i2plan_gossip_route_updates_sent_total", "Total route updates sent")
	GossipMessagesReceived = NewCounter("i2plan_gossip_messages_received_total", "Total gossip messages received")
	GossipPeersPruned      = NewCounter("i2plan_gossip_peers_pruned_total", "Total stale peers pruned")
	GossipLeavesSent       = NewCounter("i2plan_gossip_leaves_sent_total", "Total leave announcements sent")
	GossipLeavesReceived   = NewCounter("i2plan_gossip_leaves_received_total", "Total leave announcements received")
	GossipPeersDiscovered  = NewCounter("i2plan_gossip_peers_discovered_total", "Total new peers discovered via gossip")

	// Handshake metrics
	HandshakesInitiated = NewCounter("i2plan_handshakes_initiated_total", "Total handshakes initiated")
	HandshakesReceived  = NewCounter("i2plan_handshakes_received_total", "Total handshakes received")
	HandshakesSucceeded = NewCounter("i2plan_handshakes_succeeded_total", "Total successful handshakes")
	HandshakesFailed    = NewCounter("i2plan_handshakes_failed_total", "Total failed handshakes")

	// Route metrics
	RoutesTotal = NewGauge("i2plan_routes_total", "Total number of routes in routing table")

	// Transport metrics
	I2PHealthy = NewGauge("i2plan_i2p_healthy", "Whether I2P connection is healthy (1=yes, 0=no)")

	// Uptime
	StartTime = NewGauge("i2plan_start_time_seconds", "Unix timestamp when the node started")

	// Rate limiting
	RateLimitRejections = NewCounter("i2plan_ratelimit_rejections_total", "Total requests rejected by rate limiting")

	// Ban list
	BannedPeersTotal = NewGauge("i2plan_banned_peers_total", "Total number of banned peers")

	// Reconnection
	ReconnectionAttempts  = NewCounter("i2plan_reconnection_attempts_total", "Total reconnection attempts")
	ReconnectionSuccesses = NewCounter("i2plan_reconnection_successes_total", "Total successful reconnections")
)

// Latency histogram buckets (in seconds) - Prometheus convention
var (
	// DefaultLatencyBuckets are suitable for most operation latencies.
	// Values: 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s
	DefaultLatencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

	// I2PLatencyBuckets are tuned for I2P operations which are slower.
	// Values: 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s, 60s, 120s
	I2PLatencyBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}
)

// Latency histograms for operation timing
var (
	// RPC operation latencies
	RPCRequestLatency = NewHistogram(
		"i2plan_rpc_request_duration_seconds",
		"RPC request latency in seconds",
		DefaultLatencyBuckets,
	)

	// Handshake latencies (slower due to I2P)
	HandshakeLatency = NewHistogram(
		"i2plan_handshake_duration_seconds",
		"Handshake operation latency in seconds",
		I2PLatencyBuckets,
	)

	// Message send latencies
	MessageSendLatency = NewHistogram(
		"i2plan_message_send_duration_seconds",
		"Message send latency in seconds",
		I2PLatencyBuckets,
	)

	// Gossip round latencies
	GossipRoundLatency = NewHistogram(
		"i2plan_gossip_round_duration_seconds",
		"Gossip round latency in seconds",
		DefaultLatencyBuckets,
	)

	// Anti-entropy sync latencies
	AntiEntropyLatency = NewHistogram(
		"i2plan_antientropy_duration_seconds",
		"Anti-entropy sync latency in seconds",
		I2PLatencyBuckets,
	)
)

// RecordStartTime records the current time as the start time.
func RecordStartTime() {
	StartTime.Set(time.Now().Unix())
}

// Timer provides a convenient way to measure operation latency.
// Usage:
//
//	timer := metrics.NewTimer(metrics.RPCRequestLatency)
//	defer timer.ObserveDuration()
//	// ... operation ...
type Timer struct {
	histogram *Histogram
	start     time.Time
}

// NewTimer creates a new timer that will record to the given histogram.
func NewTimer(h *Histogram) *Timer {
	return &Timer{
		histogram: h,
		start:     time.Now(),
	}
}

// ObserveDuration records the elapsed time since the timer was created.
// Returns the duration for convenience.
func (t *Timer) ObserveDuration() time.Duration {
	d := time.Since(t.start)
	t.histogram.Observe(d.Seconds())
	return d
}

// TimeOperation is a helper that times the execution of a function
// and records the duration to the provided histogram.
func TimeOperation(h *Histogram, fn func()) {
	start := time.Now()
	fn()
	h.Observe(time.Since(start).Seconds())
}

// TimeOperationWithError is like TimeOperation but for functions that return an error.
// The duration is recorded regardless of whether an error occurred.
func TimeOperationWithError(h *Histogram, fn func() error) error {
	start := time.Now()
	err := fn()
	h.Observe(time.Since(start).Seconds())
	return err
}
