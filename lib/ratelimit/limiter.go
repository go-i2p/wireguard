// Package ratelimit provides a simple token bucket rate limiter.
// It is used to protect sensitive endpoints like invite acceptance
// from brute-force attacks.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a token bucket rate limiter.
type Limiter struct {
	mu       sync.Mutex
	rate     float64   // tokens per second
	capacity float64   // max tokens
	tokens   float64   // current tokens
	lastTime time.Time // last refill time
}

// New creates a new rate limiter.
// rate is tokens per second, capacity is the maximum burst size.
func New(rate float64, capacity int) *Limiter {
	return &Limiter{
		rate:     rate,
		capacity: float64(capacity),
		tokens:   float64(capacity),
		lastTime: time.Now(),
	}
}

// Allow returns true if a request is allowed, consuming one token.
// Returns false if rate limit is exceeded.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// AllowN returns true if n requests are allowed.
func (l *Limiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	needed := float64(n)
	if l.tokens >= needed {
		l.tokens -= needed
		return true
	}
	return false
}

// refill adds tokens based on elapsed time. Must be called with lock held.
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastTime).Seconds()
	l.tokens += elapsed * l.rate
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}
	l.lastTime = now
}

// Tokens returns the current number of available tokens.
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	return l.tokens
}

// KeyedLimiter provides per-key rate limiting.
type KeyedLimiter struct {
	mu       sync.Mutex
	limiters map[string]*Limiter
	rate     float64
	capacity int
	cleanup  time.Duration // how long to keep idle limiters
	stopCh   chan struct{} // channel to stop the cleanup goroutine
}

// NewKeyed creates a per-key rate limiter.
func NewKeyed(rate float64, capacity int, cleanup time.Duration) *KeyedLimiter {
	kl := &KeyedLimiter{
		limiters: make(map[string]*Limiter),
		rate:     rate,
		capacity: capacity,
		cleanup:  cleanup,
		stopCh:   make(chan struct{}),
	}
	go kl.cleanupLoop()
	return kl
}

// Close stops the cleanup goroutine and releases resources.
func (kl *KeyedLimiter) Close() {
	close(kl.stopCh)
}

// Allow checks if a request for the given key is allowed.
func (kl *KeyedLimiter) Allow(key string) bool {
	kl.mu.Lock()
	limiter, ok := kl.limiters[key]
	if !ok {
		limiter = New(kl.rate, kl.capacity)
		kl.limiters[key] = limiter
	}
	kl.mu.Unlock()

	return limiter.Allow()
}

// cleanupLoop periodically removes idle limiters.
func (kl *KeyedLimiter) cleanupLoop() {
	ticker := time.NewTicker(kl.cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-kl.stopCh:
			return
		case <-ticker.C:
			kl.mu.Lock()
			now := time.Now()
			for key, limiter := range kl.limiters {
				limiter.mu.Lock()
				// Remove if idle and at full capacity
				if now.Sub(limiter.lastTime) > kl.cleanup && limiter.tokens >= limiter.capacity {
					delete(kl.limiters, key)
				}
				limiter.mu.Unlock()
			}
			kl.mu.Unlock()
		}
	}
}
