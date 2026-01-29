package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCounter(t *testing.T) {
	// Create a counter outside the default registry for testing
	c := &Counter{name: "test_counter", help: "A test counter"}

	if c.Value() != 0 {
		t.Errorf("initial value = %d, want 0", c.Value())
	}

	c.Inc()
	if c.Value() != 1 {
		t.Errorf("after Inc() = %d, want 1", c.Value())
	}

	c.Add(5)
	if c.Value() != 6 {
		t.Errorf("after Add(5) = %d, want 6", c.Value())
	}
}

func TestCounterPrometheus(t *testing.T) {
	c := &Counter{name: "test_counter", help: "A test counter"}
	c.Add(42)

	output := c.prometheus()

	if !strings.Contains(output, "# HELP test_counter A test counter") {
		t.Error("missing HELP line")
	}
	if !strings.Contains(output, "# TYPE test_counter counter") {
		t.Error("missing TYPE line")
	}
	if !strings.Contains(output, "test_counter 42") {
		t.Errorf("missing value line, got: %s", output)
	}
}

func TestGauge(t *testing.T) {
	g := &Gauge{name: "test_gauge", help: "A test gauge"}

	if g.Value() != 0 {
		t.Errorf("initial value = %d, want 0", g.Value())
	}

	g.Set(10)
	if g.Value() != 10 {
		t.Errorf("after Set(10) = %d, want 10", g.Value())
	}

	g.Inc()
	if g.Value() != 11 {
		t.Errorf("after Inc() = %d, want 11", g.Value())
	}

	g.Dec()
	if g.Value() != 10 {
		t.Errorf("after Dec() = %d, want 10", g.Value())
	}

	g.Add(-5)
	if g.Value() != 5 {
		t.Errorf("after Add(-5) = %d, want 5", g.Value())
	}
}

func TestGaugePrometheus(t *testing.T) {
	g := &Gauge{name: "test_gauge", help: "A test gauge"}
	g.Set(123)

	output := g.prometheus()

	if !strings.Contains(output, "# HELP test_gauge A test gauge") {
		t.Error("missing HELP line")
	}
	if !strings.Contains(output, "# TYPE test_gauge gauge") {
		t.Error("missing TYPE line")
	}
	if !strings.Contains(output, "test_gauge 123") {
		t.Errorf("missing value line, got: %s", output)
	}
}

func TestHistogram(t *testing.T) {
	h := &Histogram{
		name:    "test_histogram",
		help:    "A test histogram",
		buckets: []float64{0.1, 0.5, 1.0, 5.0},
		counts:  make([]uint64, 4),
	}

	h.Observe(0.05) // fits in 0.1 bucket
	h.Observe(0.3)  // fits in 0.5 bucket
	h.Observe(0.8)  // fits in 1.0 bucket
	h.Observe(3.0)  // fits in 5.0 bucket
	h.Observe(10.0) // exceeds all buckets

	output := h.prometheus()

	if !strings.Contains(output, "# HELP test_histogram A test histogram") {
		t.Error("missing HELP line")
	}
	if !strings.Contains(output, "# TYPE test_histogram histogram") {
		t.Error("missing TYPE line")
	}
	if !strings.Contains(output, `test_histogram_bucket{le="0.1"} 1`) {
		t.Errorf("wrong 0.1 bucket count, got: %s", output)
	}
	if !strings.Contains(output, "test_histogram_count 5") {
		t.Errorf("wrong count, got: %s", output)
	}
}

func TestRegistry(t *testing.T) {
	r := &Registry{
		metrics: make(map[string]metric),
	}

	c := &Counter{name: "reg_counter", help: "A counter"}
	g := &Gauge{name: "reg_gauge", help: "A gauge"}

	r.register(c)
	r.register(g)

	c.Inc()
	g.Set(42)

	output := r.Expose()

	if !strings.Contains(output, "reg_counter 1") {
		t.Errorf("missing counter in output: %s", output)
	}
	if !strings.Contains(output, "reg_gauge 42") {
		t.Errorf("missing gauge in output: %s", output)
	}
}

func TestHandler(t *testing.T) {
	// Reset default registry for clean test
	oldRegistry := defaultRegistry
	defaultRegistry = &Registry{metrics: make(map[string]metric)}
	defer func() { defaultRegistry = oldRegistry }()

	c := NewCounter("handler_test_counter", "Test counter")
	c.Add(100)

	handler := Handler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)

	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "handler_test_counter 100") {
		t.Errorf("missing counter in body: %s", body)
	}
}

func TestDefaultMetrics(t *testing.T) {
	// Just verify that default metrics exist and can be used
	PeersTotal.Set(5)
	PeersConnected.Set(3)
	MessagesReceived.Inc()
	MessagesSent.Add(10)
	RoutesTotal.Set(8)
	I2PHealthy.Set(1)
	RateLimitRejections.Inc()
	BannedPeersTotal.Set(2)
	ReconnectionAttempts.Inc()
	ReconnectionSuccesses.Inc()

	if PeersTotal.Value() != 5 {
		t.Errorf("PeersTotal = %d, want 5", PeersTotal.Value())
	}
	if PeersConnected.Value() != 3 {
		t.Errorf("PeersConnected = %d, want 3", PeersConnected.Value())
	}
}

func TestRecordStartTime(t *testing.T) {
	RecordStartTime()

	if StartTime.Value() == 0 {
		t.Error("StartTime should be non-zero after RecordStartTime()")
	}
}
