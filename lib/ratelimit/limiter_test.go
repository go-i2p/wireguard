package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiterAllow(t *testing.T) {
	// 10 tokens/sec, capacity 5
	limiter := New(10, 5)

	// Should allow 5 requests immediately
	for i := 0; i < 5; i++ {
		if !limiter.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// 6th request should be denied
	if limiter.Allow() {
		t.Error("6th request should be denied")
	}
}

func TestLimiterRefill(t *testing.T) {
	// 100 tokens/sec, capacity 10
	limiter := New(100, 10)

	// Drain all tokens
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	// Should be empty
	if limiter.Allow() {
		t.Error("should be empty")
	}

	// Wait for refill (100ms should add ~10 tokens)
	time.Sleep(100 * time.Millisecond)

	// Should have tokens again
	if !limiter.Allow() {
		t.Error("should have tokens after refill")
	}
}

func TestLimiterAllowN(t *testing.T) {
	limiter := New(10, 10)

	// Should allow 5 at once
	if !limiter.AllowN(5) {
		t.Error("should allow 5 requests")
	}

	// Should allow another 5
	if !limiter.AllowN(5) {
		t.Error("should allow 5 more requests")
	}

	// Should deny 1
	if limiter.AllowN(1) {
		t.Error("should deny after capacity reached")
	}
}

func TestLimiterTokens(t *testing.T) {
	limiter := New(10, 5)
	tokens := limiter.Tokens()
	if tokens != 5 {
		t.Errorf("expected 5 tokens, got %f", tokens)
	}

	limiter.Allow()
	tokens = limiter.Tokens()
	if tokens < 3.9 || tokens > 4.1 {
		t.Errorf("expected ~4 tokens, got %f", tokens)
	}
}

func TestLimiterConcurrent(t *testing.T) {
	limiter := New(1000, 100)

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// Launch 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- limiter.Allow()
		}()
	}

	wg.Wait()
	close(allowed)

	// Count allowed requests
	count := 0
	for a := range allowed {
		if a {
			count++
		}
	}

	// Should have allowed approximately 100 (allowing for minor timing variance)
	if count < 99 || count > 105 {
		t.Errorf("expected ~100 allowed, got %d", count)
	}
}
