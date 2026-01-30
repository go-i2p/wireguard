package web

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDefaultRateLimitConfig(t *testing.T) {
	cfg := DefaultRateLimitConfig()

	if cfg.RequestsPerSecond != 10.0 {
		t.Errorf("RequestsPerSecond = %f, want 10.0", cfg.RequestsPerSecond)
	}
	if cfg.BurstSize != 30 {
		t.Errorf("BurstSize = %d, want 30", cfg.BurstSize)
	}
	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("CleanupInterval = %v, want 5m", cfg.CleanupInterval)
	}
}

func TestNewRateLimiter_Defaults(t *testing.T) {
	// Test with zero/negative values - should use defaults
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerSecond: 0,
		BurstSize:         -1,
		CleanupInterval:   0,
	})
	defer rl.Close()

	// Should not panic and should work
	if rl.limiter == nil {
		t.Error("limiter should not be nil")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := NewRateLimiter(RateLimitConfig{
			RequestsPerSecond: 100,
			BurstSize:         100,
			CleanupInterval:   time.Minute,
		})
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.Middleware(handler)

		for i := 0; i < 10; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: status = %d, want %d", i, w.Code, http.StatusOK)
			}
		}
	})

	t.Run("rejects requests over limit", func(t *testing.T) {
		rl := NewRateLimiter(RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         2,
			CleanupInterval:   time.Minute,
		})
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.Middleware(handler)

		// First two should succeed (burst)
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("burst request %d: status = %d, want %d", i, w.Code, http.StatusOK)
			}
		}

		// Third should be rate limited
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("over-limit request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
		}

		// Should have Retry-After header
		if w.Header().Get("Retry-After") == "" {
			t.Error("expected Retry-After header")
		}
	})

	t.Run("different IPs have separate limits", func(t *testing.T) {
		rl := NewRateLimiter(RateLimitConfig{
			RequestsPerSecond: 1,
			BurstSize:         1,
			CleanupInterval:   time.Minute,
		})
		defer rl.Close()

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := rl.Middleware(handler)

		// First IP uses its limit
		req1 := httptest.NewRequest("GET", "/", nil)
		req1.RemoteAddr = "192.168.1.1:12345"
		w1 := httptest.NewRecorder()
		wrapped.ServeHTTP(w1, req1)

		if w1.Code != http.StatusOK {
			t.Errorf("first IP: status = %d, want %d", w1.Code, http.StatusOK)
		}

		// Second IP should still work
		req2 := httptest.NewRequest("GET", "/", nil)
		req2.RemoteAddr = "192.168.1.2:12345"
		w2 := httptest.NewRecorder()
		wrapped.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("second IP: status = %d, want %d", w2.Code, http.StatusOK)
		}
	})
}

func TestRateLimiter_OnReject(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         1,
		CleanupInterval:   time.Minute,
	})
	defer rl.Close()

	var rejectCount int32
	var lastIP, lastPath string

	rl.SetOnReject(func(ip, path string) {
		atomic.AddInt32(&rejectCount, 1)
		lastIP = ip
		lastPath = path
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := rl.Middleware(handler)

	// Use up the limit
	req1 := httptest.NewRequest("GET", "/api/test", nil)
	req1.RemoteAddr = "10.0.0.1:5000"
	w1 := httptest.NewRecorder()
	wrapped.ServeHTTP(w1, req1)

	// This should trigger onReject
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	req2.RemoteAddr = "10.0.0.1:5000"
	w2 := httptest.NewRecorder()
	wrapped.ServeHTTP(w2, req2)

	if atomic.LoadInt32(&rejectCount) != 1 {
		t.Errorf("rejectCount = %d, want 1", rejectCount)
	}
	if lastIP != "10.0.0.1" {
		t.Errorf("lastIP = %q, want %q", lastIP, "10.0.0.1")
	}
	if lastPath != "/api/test" {
		t.Errorf("lastPath = %q, want %q", lastPath, "/api/test")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		wantIP     string
	}{
		{
			name:       "simple remote addr",
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "remote addr without port",
			remoteAddr: "192.168.1.1",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "127.0.0.1:80",
			xff:        "203.0.113.195",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "127.0.0.1:80",
			xff:        "203.0.113.195, 70.41.3.18, 150.172.238.178",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For with spaces",
			remoteAddr: "127.0.0.1:80",
			xff:        "  203.0.113.195  , 70.41.3.18",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:80",
			xri:        "203.0.113.195",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			remoteAddr: "127.0.0.1:80",
			xff:        "10.0.0.1",
			xri:        "10.0.0.2",
			wantIP:     "10.0.0.1",
		},
		{
			name:       "IPv6 address",
			remoteAddr: "[::1]:12345",
			wantIP:     "::1",
		},
		{
			name:       "X-Forwarded-For IPv6",
			remoteAddr: "127.0.0.1:80",
			xff:        "2001:db8::1",
			wantIP:     "2001:db8::1",
		},
		{
			name:       "invalid X-Forwarded-For falls back to remote",
			remoteAddr: "192.168.1.1:80",
			xff:        "not-an-ip",
			wantIP:     "192.168.1.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xri != "" {
				req.Header.Set("X-Real-IP", tc.xri)
			}

			got := extractIP(req)
			if got != tc.wantIP {
				t.Errorf("extractIP() = %q, want %q", got, tc.wantIP)
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{" hello", "hello"},
		{"hello ", "hello"},
		{" hello ", "hello"},
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
		{"", ""},
		{"   ", ""},
	}

	for _, tc := range tests {
		got := trimSpace(tc.input)
		if got != tc.want {
			t.Errorf("trimSpace(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseFirstIP(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "192.168.1.1"},
		{"192.168.1.1, 10.0.0.1", "192.168.1.1"},
		{" 192.168.1.1 , 10.0.0.1", "192.168.1.1"},
		{"invalid", ""},
		{"invalid, 10.0.0.1", ""},
		{"2001:db8::1", "2001:db8::1"},
		{"2001:db8::1, ::1", "2001:db8::1"},
	}

	for _, tc := range tests {
		got := parseFirstIP(tc.input)
		if got != tc.want {
			t.Errorf("parseFirstIP(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
