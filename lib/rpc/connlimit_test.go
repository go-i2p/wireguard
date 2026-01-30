package rpc

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectionLimiter_AcquireRelease(t *testing.T) {
	t.Run("acquire within limit", func(t *testing.T) {
		cl := NewConnectionLimiter(5)

		for i := 0; i < 5; i++ {
			if !cl.Acquire() {
				t.Errorf("Acquire() %d failed, should succeed", i)
			}
		}

		if cl.ActiveConnections() != 5 {
			t.Errorf("ActiveConnections() = %d, want 5", cl.ActiveConnections())
		}
	})

	t.Run("acquire at limit fails", func(t *testing.T) {
		cl := NewConnectionLimiter(2)

		cl.Acquire()
		cl.Acquire()

		if cl.Acquire() {
			t.Error("Acquire() should fail at limit")
		}
	})

	t.Run("release allows new acquire", func(t *testing.T) {
		cl := NewConnectionLimiter(1)

		cl.Acquire()
		if cl.Acquire() {
			t.Error("second Acquire() should fail")
		}

		cl.Release()

		if !cl.Acquire() {
			t.Error("Acquire() after Release() should succeed")
		}
	})
}

func TestConnectionLimiter_DefaultMax(t *testing.T) {
	cl := NewConnectionLimiter(0)
	if cl.MaxConnections() != DefaultMaxConnections {
		t.Errorf("MaxConnections() = %d, want %d", cl.MaxConnections(), DefaultMaxConnections)
	}

	cl2 := NewConnectionLimiter(-5)
	if cl2.MaxConnections() != DefaultMaxConnections {
		t.Errorf("MaxConnections() with negative = %d, want %d", cl2.MaxConnections(), DefaultMaxConnections)
	}
}

func TestConnectionLimiter_SetMaxConnections(t *testing.T) {
	cl := NewConnectionLimiter(10)

	cl.SetMaxConnections(50)
	if cl.MaxConnections() != 50 {
		t.Errorf("MaxConnections() = %d, want 50", cl.MaxConnections())
	}

	cl.SetMaxConnections(0)
	if cl.MaxConnections() != DefaultMaxConnections {
		t.Errorf("SetMaxConnections(0) should use default, got %d", cl.MaxConnections())
	}
}

func TestConnectionLimiter_OnReject(t *testing.T) {
	cl := NewConnectionLimiter(1)
	var rejectedCount int32

	cl.SetOnReject(func(addr net.Addr) {
		atomic.AddInt32(&rejectedCount, 1)
	})

	// Use up the slot
	cl.Acquire()

	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()

	// This should trigger the reject callback
	result := cl.TryAccept(client)

	if result != nil {
		t.Error("TryAccept should return nil when limit reached")
	}

	if atomic.LoadInt32(&rejectedCount) != 1 {
		t.Errorf("onReject callback should be called once, got %d", rejectedCount)
	}
}

func TestConnectionLimiter_TryAccept(t *testing.T) {
	t.Run("accepts within limit", func(t *testing.T) {
		cl := NewConnectionLimiter(5)

		server, client := net.Pipe()
		defer server.Close()

		result := cl.TryAccept(client)
		if result == nil {
			t.Error("TryAccept should succeed within limit")
		}

		if cl.ActiveConnections() != 1 {
			t.Errorf("ActiveConnections() = %d, want 1", cl.ActiveConnections())
		}

		result.Close()
	})

	t.Run("rejects at limit", func(t *testing.T) {
		cl := NewConnectionLimiter(1)

		// Fill the limit
		server1, client1 := net.Pipe()
		defer server1.Close()
		cl.TryAccept(client1)

		// This should be rejected
		server2, client2 := net.Pipe()
		defer server2.Close()

		result := cl.TryAccept(client2)
		if result != nil {
			t.Error("TryAccept should return nil when limit reached")
		}
	})
}

func TestLimitedConn_Close(t *testing.T) {
	cl := NewConnectionLimiter(5)

	server, client := net.Pipe()
	defer server.Close()

	cl.Acquire()
	wrapped := cl.WrapConn(client)

	if cl.ActiveConnections() != 1 {
		t.Errorf("before close: ActiveConnections() = %d, want 1", cl.ActiveConnections())
	}

	wrapped.Close()

	if cl.ActiveConnections() != 0 {
		t.Errorf("after close: ActiveConnections() = %d, want 0", cl.ActiveConnections())
	}

	// Double close should be safe
	wrapped.Close()
	if cl.ActiveConnections() != 0 {
		t.Error("double close should not decrement below 0")
	}
}

func TestConnectionLimiter_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent test in short mode")
	}

	cl := NewConnectionLimiter(50)
	var wg sync.WaitGroup
	var acquired int32
	var failed int32

	// Try to acquire 100 connections concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cl.Acquire() {
				atomic.AddInt32(&acquired, 1)
				time.Sleep(10 * time.Millisecond)
				cl.Release()
			} else {
				atomic.AddInt32(&failed, 1)
			}
		}()
	}

	wg.Wait()

	// With 50 slots and 100 concurrent requests, some should succeed and some should fail
	// The exact numbers depend on timing, but we should see both
	if acquired == 0 {
		t.Error("no connections were acquired")
	}
	if failed == 0 {
		t.Error("no connections were rejected")
	}

	t.Logf("acquired: %d, failed: %d", acquired, failed)
}

func TestConnectionLimiter_Constants(t *testing.T) {
	if DefaultMaxConnections != 100 {
		t.Errorf("DefaultMaxConnections = %d, want 100", DefaultMaxConnections)
	}

	if ErrTooManyConnections.Error() != "too many connections" {
		t.Errorf("ErrTooManyConnections.Error() = %q", ErrTooManyConnections.Error())
	}
}
