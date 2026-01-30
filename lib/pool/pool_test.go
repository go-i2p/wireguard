package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockConn is a mock connection for testing.
type mockConn struct {
	id       int
	closed   bool
	mu       sync.Mutex
	closedAt time.Time
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.closedAt = time.Now()
	return nil
}

func (m *mockConn) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// mockFactory creates mock connections.
func mockFactory(counter *int32) Factory {
	return func(ctx context.Context) (Connection, error) {
		id := atomic.AddInt32(counter, 1)
		return &mockConn{id: int(id)}, nil
	}
}

// failingFactory returns errors.
func failingFactory() Factory {
	return func(ctx context.Context) (Connection, error) {
		return nil, errors.New("connection failed")
	}
}

func TestPoolAcquireRelease(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 3
	cfg.HealthCheckInterval = 0 // disable for this test

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	// Acquire a connection
	conn1, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}
	if conn1 == nil {
		t.Fatal("Expected non-nil connection")
	}

	stats := p.Stats()
	if stats.NumOpen != 1 {
		t.Errorf("Expected 1 open, got %d", stats.NumOpen)
	}
	if stats.NumIdle != 0 {
		t.Errorf("Expected 0 idle, got %d", stats.NumIdle)
	}
	if stats.NumInUse != 1 {
		t.Errorf("Expected 1 in use, got %d", stats.NumInUse)
	}

	// Release the connection
	p.Release(conn1)

	stats = p.Stats()
	if stats.NumOpen != 1 {
		t.Errorf("Expected 1 open after release, got %d", stats.NumOpen)
	}
	if stats.NumIdle != 1 {
		t.Errorf("Expected 1 idle after release, got %d", stats.NumIdle)
	}
	if stats.NumInUse != 0 {
		t.Errorf("Expected 0 in use after release, got %d", stats.NumInUse)
	}

	// Acquire again - should get same connection
	conn2, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Second acquire failed: %v", err)
	}
	if conn2 != conn1 {
		t.Error("Expected to get same connection from pool")
	}
}

func TestPoolMaxSize(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 2
	cfg.AcquireTimeout = 100 * time.Millisecond
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	// Acquire two connections
	conn1, _ := p.Acquire(context.Background())
	conn2, _ := p.Acquire(context.Background())

	if counter != 2 {
		t.Errorf("Expected 2 connections created, got %d", counter)
	}

	// Third acquire should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Acquire(ctx)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected timeout error, got %v", err)
	}

	// Release one and try again
	p.Release(conn1)

	conn3, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Third acquire after release failed: %v", err)
	}
	if conn3 != conn1 {
		t.Error("Expected to get released connection")
	}

	p.Release(conn2)
	p.Release(conn3)
}

func TestPoolFactoryError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 2
	cfg.HealthCheckInterval = 0

	p := New(failingFactory(), cfg)
	defer p.Close()

	_, err := p.Acquire(context.Background())
	if err == nil {
		t.Error("Expected error from factory, got nil")
	}

	stats := p.Stats()
	if stats.NumOpen != 0 {
		t.Errorf("Expected 0 open after failed create, got %d", stats.NumOpen)
	}
	if stats.AcquireFailed != 1 {
		t.Errorf("Expected 1 acquire failure, got %d", stats.AcquireFailed)
	}
}

func TestPoolClose(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 3
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)

	// Acquire and release some connections
	conn1, _ := p.Acquire(context.Background())
	conn2, _ := p.Acquire(context.Background())
	p.Release(conn1)
	p.Release(conn2)

	// Close the pool
	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait a bit for async closes
	time.Sleep(10 * time.Millisecond)

	// Verify connections are closed
	if !conn1.(*mockConn).IsClosed() {
		t.Error("conn1 should be closed")
	}
	if !conn2.(*mockConn).IsClosed() {
		t.Error("conn2 should be closed")
	}

	// Acquire after close should fail
	_, err := p.Acquire(context.Background())
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}

	// Double close should return error
	if err := p.Close(); err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed on double close, got %v", err)
	}
}

func TestPoolDiscard(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 2
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	conn, _ := p.Acquire(context.Background())
	stats := p.Stats()
	if stats.NumOpen != 1 {
		t.Errorf("Expected 1 open, got %d", stats.NumOpen)
	}

	// Discard the connection
	p.Discard(conn)

	// Wait for async close
	time.Sleep(10 * time.Millisecond)

	stats = p.Stats()
	if stats.NumOpen != 0 {
		t.Errorf("Expected 0 open after discard, got %d", stats.NumOpen)
	}
	if !conn.(*mockConn).IsClosed() {
		t.Error("Discarded connection should be closed")
	}
}

func TestPoolIdleTimeout(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 2
	cfg.MaxIdleTime = 50 * time.Millisecond
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	conn, _ := p.Acquire(context.Background())
	p.Release(conn)

	// Wait for idle timeout
	time.Sleep(100 * time.Millisecond)

	// Acquire should create new connection (old one is stale)
	conn2, _ := p.Acquire(context.Background())
	if conn2 == conn {
		t.Error("Should get new connection after idle timeout")
	}

	// Old connection should be closed
	time.Sleep(10 * time.Millisecond)
	if !conn.(*mockConn).IsClosed() {
		t.Error("Stale connection should be closed")
	}
}

func TestPoolHealthCheck(t *testing.T) {
	var counter int32
	var healthCheckCount int32
	healthyConns := make(map[*mockConn]bool)
	var mu sync.Mutex

	cfg := DefaultConfig()
	cfg.MaxSize = 2
	cfg.HealthCheckInterval = 0 // disable background check, we'll check on acquire
	cfg.HealthCheck = func(conn Connection) bool {
		atomic.AddInt32(&healthCheckCount, 1)
		mc := conn.(*mockConn)
		mu.Lock()
		defer mu.Unlock()
		healthy, exists := healthyConns[mc]
		if !exists {
			return true // default to healthy
		}
		return healthy
	}

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	conn, _ := p.Acquire(context.Background())
	mc := conn.(*mockConn)

	// Mark as unhealthy
	mu.Lock()
	healthyConns[mc] = false
	mu.Unlock()

	p.Release(conn)

	// Acquire should skip unhealthy connection
	conn2, _ := p.Acquire(context.Background())
	if conn2 == conn {
		t.Error("Should not get unhealthy connection")
	}

	// Old connection should be closed
	time.Sleep(10 * time.Millisecond)
	if !mc.IsClosed() {
		t.Error("Unhealthy connection should be closed")
	}
}

func TestPoolConcurrentAcquireRelease(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 5
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	var wg sync.WaitGroup
	numWorkers := 20
	opsPerWorker := 10

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				conn, err := p.Acquire(context.Background())
				if err != nil {
					t.Errorf("Acquire failed: %v", err)
					return
				}
				// Simulate some work
				time.Sleep(time.Millisecond)
				p.Release(conn)
			}
		}()
	}

	wg.Wait()

	stats := p.Stats()
	if stats.AcquireSuccess != uint64(numWorkers*opsPerWorker) {
		t.Errorf("Expected %d successful acquires, got %d",
			numWorkers*opsPerWorker, stats.AcquireSuccess)
	}
	if stats.AcquireFailed != 0 {
		t.Errorf("Expected 0 failed acquires, got %d", stats.AcquireFailed)
	}
}

func TestPoolContextCancellation(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 1
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	// Acquire the only connection
	conn, _ := p.Acquire(context.Background())

	// Cancel the context while waiting
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	var acquireErr error
	go func() {
		defer wg.Done()
		_, acquireErr = p.Acquire(ctx)
	}()

	// Give the goroutine time to start waiting
	time.Sleep(20 * time.Millisecond)
	cancel()

	wg.Wait()

	if acquireErr == nil {
		t.Error("Expected error on cancelled context")
	}
	if !errors.Is(acquireErr, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", acquireErr)
	}

	p.Release(conn)
}

func TestPoolStats(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.MaxSize = 5
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	stats := p.Stats()
	if stats.MaxSize != 5 {
		t.Errorf("Expected MaxSize 5, got %d", stats.MaxSize)
	}
	if stats.NumOpen != 0 {
		t.Errorf("Expected 0 open, got %d", stats.NumOpen)
	}

	conn1, _ := p.Acquire(context.Background())
	conn2, _ := p.Acquire(context.Background())
	p.Release(conn1)

	stats = p.Stats()
	if stats.NumOpen != 2 {
		t.Errorf("Expected 2 open, got %d", stats.NumOpen)
	}
	if stats.NumIdle != 1 {
		t.Errorf("Expected 1 idle, got %d", stats.NumIdle)
	}
	if stats.NumInUse != 1 {
		t.Errorf("Expected 1 in use, got %d", stats.NumInUse)
	}
	if stats.AcquireCount != 2 {
		t.Errorf("Expected 2 acquire count, got %d", stats.AcquireCount)
	}
	if stats.AcquireSuccess != 2 {
		t.Errorf("Expected 2 acquire success, got %d", stats.AcquireSuccess)
	}
	if stats.ReleaseCount != 1 {
		t.Errorf("Expected 1 release count, got %d", stats.ReleaseCount)
	}

	p.Release(conn2)
}

func TestPoolReleaseNil(t *testing.T) {
	var counter int32
	cfg := DefaultConfig()
	cfg.HealthCheckInterval = 0

	p := New(mockFactory(&counter), cfg)
	defer p.Close()

	// Should not panic
	p.Release(nil)
	p.Discard(nil)
}

func TestPoolBackgroundHealthCheck(t *testing.T) {
	var counter int32
	var healthCheckCount int32
	healthyConns := sync.Map{}

	cfg := DefaultConfig()
	cfg.MaxSize = 3
	cfg.MaxIdleTime = 1 * time.Hour // don't expire by time
	cfg.HealthCheckInterval = 50 * time.Millisecond
	cfg.HealthCheck = func(conn Connection) bool {
		atomic.AddInt32(&healthCheckCount, 1)
		mc := conn.(*mockConn)
		healthy, exists := healthyConns.Load(mc)
		if !exists {
			return true
		}
		return healthy.(bool)
	}

	p := New(mockFactory(&counter), cfg)

	// Acquire and release connections
	conn1, _ := p.Acquire(context.Background())
	conn2, _ := p.Acquire(context.Background())
	p.Release(conn1)
	p.Release(conn2)

	// Mark one as unhealthy
	healthyConns.Store(conn1.(*mockConn), false)

	// Wait for health check
	time.Sleep(100 * time.Millisecond)

	p.Close()

	// Check that health check ran
	if atomic.LoadInt32(&healthCheckCount) == 0 {
		t.Error("Health check should have run")
	}

	// Unhealthy connection should be closed
	if !conn1.(*mockConn).IsClosed() {
		t.Error("Unhealthy connection should be closed after health check")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxSize != 10 {
		t.Errorf("Expected default MaxSize 10, got %d", cfg.MaxSize)
	}
	if cfg.MaxIdleTime != 10*time.Minute {
		t.Errorf("Expected default MaxIdleTime 10m, got %v", cfg.MaxIdleTime)
	}
	if cfg.AcquireTimeout != 30*time.Second {
		t.Errorf("Expected default AcquireTimeout 30s, got %v", cfg.AcquireTimeout)
	}
	if cfg.HealthCheckInterval != 1*time.Minute {
		t.Errorf("Expected default HealthCheckInterval 1m, got %v", cfg.HealthCheckInterval)
	}
}

func TestUpdateMetrics(t *testing.T) {
	stats := Stats{
		MaxSize:  10,
		NumOpen:  5,
		NumIdle:  3,
		NumInUse: 2,
	}

	// Should not panic
	UpdateMetrics(stats)

	// Verify metrics were set
	if PoolConnectionsTotal.Value() != 10 {
		t.Errorf("Expected PoolConnectionsTotal 10, got %d", PoolConnectionsTotal.Value())
	}
	if PoolConnectionsOpen.Value() != 5 {
		t.Errorf("Expected PoolConnectionsOpen 5, got %d", PoolConnectionsOpen.Value())
	}
	if PoolConnectionsIdle.Value() != 3 {
		t.Errorf("Expected PoolConnectionsIdle 3, got %d", PoolConnectionsIdle.Value())
	}
	if PoolConnectionsInUse.Value() != 2 {
		t.Errorf("Expected PoolConnectionsInUse 2, got %d", PoolConnectionsInUse.Value())
	}
}
