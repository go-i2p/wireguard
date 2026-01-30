// Package web provides rate limiting middleware for the web UI.
package web

import (
	"net"
	"net/http"
	"time"

	"github.com/go-i2p/wireguard/lib/ratelimit"
)

// RateLimitConfig configures rate limiting for web endpoints.
type RateLimitConfig struct {
	// RequestsPerSecond is the rate of allowed requests per IP.
	RequestsPerSecond float64
	// BurstSize is the maximum burst size per IP.
	BurstSize int
	// CleanupInterval is how often to clean up idle limiters.
	CleanupInterval time.Duration
}

// DefaultRateLimitConfig returns sensible defaults for rate limiting.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 10.0, // 10 requests per second per IP
		BurstSize:         30,   // Allow bursts of up to 30 requests
		CleanupInterval:   5 * time.Minute,
	}
}

// RateLimiter provides HTTP middleware for per-IP rate limiting.
type RateLimiter struct {
	limiter  *ratelimit.KeyedLimiter
	onReject func(ip string, path string) // optional callback for rejections
}

// NewRateLimiter creates a new rate limiter with the given configuration.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.RequestsPerSecond <= 0 {
		cfg.RequestsPerSecond = 10.0
	}
	if cfg.BurstSize <= 0 {
		cfg.BurstSize = 30
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}

	return &RateLimiter{
		limiter: ratelimit.NewKeyed(cfg.RequestsPerSecond, cfg.BurstSize, cfg.CleanupInterval),
	}
}

// SetOnReject sets a callback that is invoked when a request is rate limited.
func (rl *RateLimiter) SetOnReject(fn func(ip string, path string)) {
	rl.onReject = fn
}

// Close stops the rate limiter's cleanup goroutine.
func (rl *RateLimiter) Close() {
	rl.limiter.Close()
}

// Middleware returns an HTTP middleware that enforces rate limiting.
// Requests that exceed the rate limit receive a 429 Too Many Requests response.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)

		if !rl.limiter.Allow(ip) {
			if rl.onReject != nil {
				rl.onReject(ip, r.URL.Path)
			}

			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractIP extracts the client IP from the request.
// It checks X-Forwarded-For and X-Real-IP headers first (for reverse proxy setups),
// then falls back to the remote address.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For header (common for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		if ip := parseFirstIP(xff); ip != "" {
			return ip
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// parseFirstIP extracts the first valid IP from a comma-separated list.
func parseFirstIP(xff string) string {
	for i := 0; i < len(xff); i++ {
		if xff[i] == ',' {
			candidate := xff[:i]
			// Trim whitespace
			candidate = trimSpace(candidate)
			if ip := net.ParseIP(candidate); ip != nil {
				return ip.String()
			}
			return ""
		}
	}
	// No comma found, treat entire string as IP
	candidate := trimSpace(xff)
	if ip := net.ParseIP(candidate); ip != nil {
		return ip.String()
	}
	return ""
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
