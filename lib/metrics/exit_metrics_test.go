package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestNewExitMetrics(t *testing.T) {
	m := NewExitMetrics()

	if m.ActiveClients() != 0 {
		t.Errorf("expected 0 active clients, got %d", m.ActiveClients())
	}
	if m.BytesForwarded() != 0 {
		t.Errorf("expected 0 bytes forwarded, got %d", m.BytesForwarded())
	}
	if m.DroppedPackets() != 0 {
		t.Errorf("expected 0 dropped packets, got %d", m.DroppedPackets())
	}
	if m.Uptime() > time.Second {
		t.Errorf("uptime should be less than 1 second, got %v", m.Uptime())
	}
}

func TestNewExitMetricsCollector(t *testing.T) {
	tests := []struct {
		name            string
		bandwidthWindow time.Duration
		expectedWindow  time.Duration
	}{
		{
			name:            "valid window",
			bandwidthWindow: 30 * time.Second,
			expectedWindow:  30 * time.Second,
		},
		{
			name:            "zero window defaults to 10s",
			bandwidthWindow: 0,
			expectedWindow:  10 * time.Second,
		},
		{
			name:            "negative window defaults to 10s",
			bandwidthWindow: -5 * time.Second,
			expectedWindow:  10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewExitMetricsCollector(tt.bandwidthWindow)
			if c == nil {
				t.Fatal("expected non-nil collector")
			}
			if c.bandwidthWindow != tt.expectedWindow {
				t.Errorf("expected bandwidth window %v, got %v", tt.expectedWindow, c.bandwidthWindow)
			}
			if c.metrics == nil {
				t.Error("expected non-nil metrics")
			}
			if c.connStats == nil {
				t.Error("expected non-nil connStats map")
			}
		})
	}
}

func TestExitMetricsCollector_AddRemoveConnection(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)

	// Add first connection
	c.AddConnection("client1")
	if c.metrics.ActiveClients() != 1 {
		t.Errorf("expected 1 active client, got %d", c.metrics.ActiveClients())
	}

	// Add second connection
	c.AddConnection("client2")
	if c.metrics.ActiveClients() != 2 {
		t.Errorf("expected 2 active clients, got %d", c.metrics.ActiveClients())
	}

	// Add duplicate connection (should not increase count)
	c.AddConnection("client1")
	if c.metrics.ActiveClients() != 2 {
		t.Errorf("expected 2 active clients after duplicate, got %d", c.metrics.ActiveClients())
	}

	// Remove connection
	c.RemoveConnection("client1")
	if c.metrics.ActiveClients() != 1 {
		t.Errorf("expected 1 active client after removal, got %d", c.metrics.ActiveClients())
	}

	// Remove non-existent connection (should not decrease count)
	c.RemoveConnection("client3")
	if c.metrics.ActiveClients() != 1 {
		t.Errorf("expected 1 active client after removing non-existent, got %d", c.metrics.ActiveClients())
	}

	// Remove last connection
	c.RemoveConnection("client2")
	if c.metrics.ActiveClients() != 0 {
		t.Errorf("expected 0 active clients, got %d", c.metrics.ActiveClients())
	}
}

func TestExitMetricsCollector_RecordBytesSent(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	c.RecordBytesSent("client1", 1024)
	if c.metrics.BytesForwarded() != 1024 {
		t.Errorf("expected 1024 bytes forwarded, got %d", c.metrics.BytesForwarded())
	}

	c.RecordBytesSent("client1", 2048)
	if c.metrics.BytesForwarded() != 3072 {
		t.Errorf("expected 3072 bytes forwarded, got %d", c.metrics.BytesForwarded())
	}

	// Verify per-connection stats
	stats := c.GetConnectionStats()
	if stats["client1"].BytesSent != 3072 {
		t.Errorf("expected client1 to have sent 3072 bytes, got %d", stats["client1"].BytesSent)
	}
}

func TestExitMetricsCollector_RecordBytesReceived(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	c.RecordBytesReceived("client1", 512)
	if c.metrics.BytesReceived() != 512 {
		t.Errorf("expected 512 bytes received, got %d", c.metrics.BytesReceived())
	}

	c.RecordBytesReceived("client1", 256)
	if c.metrics.BytesReceived() != 768 {
		t.Errorf("expected 768 bytes received, got %d", c.metrics.BytesReceived())
	}

	// Verify per-connection stats
	stats := c.GetConnectionStats()
	if stats["client1"].BytesReceived != 768 {
		t.Errorf("expected client1 to have received 768 bytes, got %d", stats["client1"].BytesReceived)
	}
}

func TestExitMetricsCollector_RecordPacketDrop(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	c.RecordPacketDrop("client1")
	if c.metrics.DroppedPackets() != 1 {
		t.Errorf("expected 1 dropped packet, got %d", c.metrics.DroppedPackets())
	}

	c.RecordPacketDrop("client1")
	c.RecordPacketDrop("client1")
	if c.metrics.DroppedPackets() != 3 {
		t.Errorf("expected 3 dropped packets, got %d", c.metrics.DroppedPackets())
	}

	// Verify per-connection stats
	stats := c.GetConnectionStats()
	if stats["client1"].PacketsDropped != 3 {
		t.Errorf("expected client1 to have 3 dropped packets, got %d", stats["client1"].PacketsDropped)
	}
}

func TestExitMetricsCollector_RecordLatency(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	// Record some latency samples
	c.RecordLatency("client1", 10*time.Millisecond)
	c.RecordLatency("client1", 20*time.Millisecond)
	c.RecordLatency("client1", 30*time.Millisecond)

	avgLatency := c.metrics.AverageLatency()
	expected := 20 * time.Millisecond
	if avgLatency != expected {
		t.Errorf("expected average latency %v, got %v", expected, avgLatency)
	}

	// Verify latency samples are stored
	stats := c.GetConnectionStats()
	if len(stats["client1"].LatencySamples) != 3 {
		t.Errorf("expected 3 latency samples, got %d", len(stats["client1"].LatencySamples))
	}
}

func TestExitMetricsCollector_RecordLatency_MaxSamples(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	// Record more than 100 samples to test cap
	for i := 0; i < 150; i++ {
		c.RecordLatency("client1", time.Duration(i)*time.Millisecond)
	}

	stats := c.GetConnectionStats()
	if len(stats["client1"].LatencySamples) != 100 {
		t.Errorf("expected 100 latency samples (capped), got %d", len(stats["client1"].LatencySamples))
	}
}

func TestExitMetricsCollector_CalculateBandwidth(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	// Record some traffic
	c.RecordBytesSent("client1", 1_000_000)   // 1 MB sent
	c.RecordBytesReceived("client1", 500_000) // 0.5 MB received

	// Wait a bit to get measurable time difference
	time.Sleep(100 * time.Millisecond)

	// Calculate bandwidth
	c.CalculateBandwidth()

	bw := c.metrics.CurrentBandwidthMbps()
	// Should be > 0 since we transferred data
	if bw <= 0 {
		t.Errorf("expected bandwidth > 0, got %f", bw)
	}
}

func TestExitMetricsCollector_CalculateBandwidth_ZeroTime(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)

	// Calculate immediately without any time passing
	c.CalculateBandwidth()

	bw := c.metrics.CurrentBandwidthMbps()
	// Should be 0 or very small
	if bw < 0 {
		t.Errorf("expected non-negative bandwidth, got %f", bw)
	}
}

func TestExitMetrics_Load(t *testing.T) {
	tests := []struct {
		name            string
		activeClients   int64
		bandwidthMbps   float64
		expectedLoadMin float64
		expectedLoadMax float64
	}{
		{
			name:            "no load",
			activeClients:   0,
			bandwidthMbps:   0,
			expectedLoadMin: 0.0,
			expectedLoadMax: 0.0,
		},
		{
			name:            "medium load - bandwidth",
			activeClients:   10,
			bandwidthMbps:   500, // 50% of 1Gbps
			expectedLoadMin: 0.30,
			expectedLoadMax: 0.40,
		},
		{
			name:            "medium load - clients",
			activeClients:   50, // 50% of 100
			bandwidthMbps:   100,
			expectedLoadMin: 0.15,
			expectedLoadMax: 0.25,
		},
		{
			name:            "high load",
			activeClients:   80,
			bandwidthMbps:   800,
			expectedLoadMin: 0.70,
			expectedLoadMax: 0.85,
		},
		{
			name:            "overload (capped at 1.0)",
			activeClients:   200,
			bandwidthMbps:   2000,
			expectedLoadMin: 1.0,
			expectedLoadMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewExitMetrics()
			m.activeClients.Store(tt.activeClients)
			milliMbps := uint64(tt.bandwidthMbps * 1000)
			m.currentBandwidthMbps.Store(milliMbps)

			load := m.Load()
			if load < tt.expectedLoadMin || load > tt.expectedLoadMax {
				t.Errorf("expected load between %f and %f, got %f",
					tt.expectedLoadMin, tt.expectedLoadMax, load)
			}
		})
	}
}

func TestExitMetricsCollector_GetConnectionStats(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)

	c.AddConnection("client1")
	c.AddConnection("client2")
	c.RecordBytesSent("client1", 1000)
	c.RecordBytesReceived("client2", 2000)

	stats := c.GetConnectionStats()

	if len(stats) != 2 {
		t.Errorf("expected 2 connection stats, got %d", len(stats))
	}

	if stats["client1"].BytesSent != 1000 {
		t.Errorf("expected client1 sent 1000 bytes, got %d", stats["client1"].BytesSent)
	}

	if stats["client2"].BytesReceived != 2000 {
		t.Errorf("expected client2 received 2000 bytes, got %d", stats["client2"].BytesReceived)
	}

	// Verify returned map is a copy (modifying it shouldn't affect internal state)
	stats["client1"] = ConnectionStats{BytesSent: 9999}
	stats2 := c.GetConnectionStats()
	if stats2["client1"].BytesSent == 9999 {
		t.Error("GetConnectionStats should return a copy, not a reference")
	}
}

func TestExitMetricsCollector_Reset(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)

	// Add some data
	c.AddConnection("client1")
	c.RecordBytesSent("client1", 1000)
	c.RecordBytesReceived("client1", 500)
	c.RecordPacketDrop("client1")

	// Verify data exists
	if c.metrics.ActiveClients() == 0 {
		t.Fatal("expected active clients before reset")
	}

	// Reset
	c.Reset()

	// Verify everything is cleared
	if c.metrics.ActiveClients() != 0 {
		t.Errorf("expected 0 active clients after reset, got %d", c.metrics.ActiveClients())
	}
	if c.metrics.BytesForwarded() != 0 {
		t.Errorf("expected 0 bytes forwarded after reset, got %d", c.metrics.BytesForwarded())
	}
	if c.metrics.DroppedPackets() != 0 {
		t.Errorf("expected 0 dropped packets after reset, got %d", c.metrics.DroppedPackets())
	}
	if len(c.connStats) != 0 {
		t.Errorf("expected empty connStats after reset, got %d entries", len(c.connStats))
	}
}

func TestExitMetricsCollector_ConcurrentAccess(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)

	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	// Concurrent adds and removes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			clientID := "client" + string(rune('0'+id))
			for j := 0; j < operationsPerGoroutine; j++ {
				c.AddConnection(clientID)
				c.RecordBytesSent(clientID, 100)
				c.RecordBytesReceived(clientID, 50)
				c.RecordPacketDrop(clientID)
				c.RecordLatency(clientID, time.Millisecond)
				if j%10 == 0 {
					c.CalculateBandwidth()
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify metrics are consistent (no data races)
	stats := c.GetConnectionStats()
	if len(stats) > numGoroutines {
		t.Errorf("expected at most %d connections, got %d", numGoroutines, len(stats))
	}

	// Check that metrics were recorded
	if c.metrics.BytesForwarded() == 0 {
		t.Error("expected some bytes to be forwarded")
	}
}

func TestExitMetrics_Uptime(t *testing.T) {
	m := NewExitMetrics()

	// Uptime should be near zero initially
	uptime := m.Uptime()
	if uptime > time.Second {
		t.Errorf("expected uptime < 1s, got %v", uptime)
	}

	// Wait and check uptime increases
	time.Sleep(100 * time.Millisecond)
	uptime2 := m.Uptime()
	if uptime2 <= uptime {
		t.Errorf("expected uptime to increase, got %v after %v", uptime2, uptime)
	}
}

func TestExitMetrics_LastUpdate(t *testing.T) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	lastUpdate1 := c.metrics.LastUpdate()

	// Sleep to ensure time difference (100ms for reliability)
	time.Sleep(100 * time.Millisecond)

	// Update metrics
	c.RecordBytesSent("client1", 100)

	lastUpdate2 := c.metrics.LastUpdate()

	if !lastUpdate2.After(lastUpdate1) {
		t.Errorf("expected LastUpdate %v to be after %v", lastUpdate2, lastUpdate1)
	}
}

func BenchmarkExitMetricsCollector_RecordBytesSent(b *testing.B) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordBytesSent("client1", 1024)
	}
}

func BenchmarkExitMetricsCollector_RecordLatency(b *testing.B) {
	c := NewExitMetricsCollector(10 * time.Second)
	c.AddConnection("client1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordLatency("client1", 10*time.Millisecond)
	}
}

func BenchmarkExitMetrics_Load(b *testing.B) {
	m := NewExitMetrics()
	m.activeClients.Store(50)
	m.currentBandwidthMbps.Store(500000) // 500 Mbps

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Load()
	}
}
