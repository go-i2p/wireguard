// Package pool provides a generic connection pool implementation.
// It supports configurable pool size, connection timeouts, health checking,
// and metrics for monitoring pool utilization.
package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrPoolClosed is returned when operating on a closed pool.
	ErrPoolClosed = errors.New("pool: pool is closed")
	// ErrPoolExhausted is returned when no connections are available.
	ErrPoolExhausted = errors.New("pool: connection pool exhausted")
	// ErrInvalidConnection is returned when a connection fails health check.
	ErrInvalidConnection = errors.New("pool: connection is invalid")
	// ErrTimeout is returned when acquiring a connection times out.
	ErrTimeout = errors.New("pool: connection acquisition timeout")
)

// Connection represents a poolable connection.
type Connection interface {
	// Close closes the connection.
	Close() error
}

// Factory creates new connections.
type Factory func(ctx context.Context) (Connection, error)

// HealthChecker checks if a connection is still valid.
type HealthChecker func(conn Connection) bool

// Config configures the connection pool.
type Config struct {
	// MaxSize is the maximum number of connections in the pool.
	// Default: 10
	MaxSize int
	// MinIdle is the minimum number of idle connections to maintain.
	// Default: 0
	MinIdle int
	// MaxIdleTime is how long an idle connection can stay in the pool.
	// Connections idle longer than this are closed.
	// Default: 10 minutes
	MaxIdleTime time.Duration
	// AcquireTimeout is how long to wait when acquiring a connection.
	// Default: 30 seconds
	AcquireTimeout time.Duration
	// HealthCheckInterval is how often to run health checks on idle connections.
	// Set to 0 to disable periodic health checks.
	// Default: 1 minute
	HealthCheckInterval time.Duration
	// HealthCheck is a function to check if a connection is still valid.
	// If nil, no health checks are performed.
	HealthCheck HealthChecker
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxSize:             10,
		MinIdle:             0,
		MaxIdleTime:         10 * time.Minute,
		AcquireTimeout:      30 * time.Second,
		HealthCheckInterval: 1 * time.Minute,
	}
}

// pooledConn wraps a connection with metadata.
type pooledConn struct {
	conn      Connection
	createdAt time.Time
	lastUsed  time.Time
}

// Pool is a connection pool.
type Pool struct {
	factory    Factory
	config     Config
	mu         sync.Mutex
	cond       *sync.Cond
	idle       []*pooledConn
	numOpen    int
	closed     bool
	stopHealth chan struct{}
	healthDone chan struct{}

	// Metrics
	acquireCount   uint64
	acquireSuccess uint64
	acquireFailed  uint64
	releaseCount   uint64
	healthFails    uint64
}

// New creates a new connection pool.
func New(factory Factory, cfg Config) *Pool {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 10
	}
	if cfg.MaxIdleTime <= 0 {
		cfg.MaxIdleTime = 10 * time.Minute
	}
	if cfg.AcquireTimeout <= 0 {
		cfg.AcquireTimeout = 30 * time.Second
	}

	p := &Pool{
		factory:    factory,
		config:     cfg,
		idle:       make([]*pooledConn, 0, cfg.MaxSize),
		stopHealth: make(chan struct{}),
		healthDone: make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)

	if cfg.HealthCheckInterval > 0 {
		go p.healthCheckLoop()
	} else {
		close(p.healthDone)
	}

	log.WithField("maxSize", cfg.MaxSize).WithField("maxIdleTime", cfg.MaxIdleTime).Debug("pool created")
	return p
}

// Acquire gets a connection from the pool.
// It blocks until a connection is available or the context is canceled.
func (p *Pool) Acquire(ctx context.Context) (Connection, error) {
	atomic.AddUint64(&p.acquireCount, 1)

	// Use configured timeout if context has no deadline
	acquireCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && p.config.AcquireTimeout > 0 {
		var cancel context.CancelFunc
		acquireCtx, cancel = context.WithTimeout(ctx, p.config.AcquireTimeout)
		defer cancel()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if p.closed {
			atomic.AddUint64(&p.acquireFailed, 1)
			return nil, ErrPoolClosed
		}

		// Check context cancellation
		select {
		case <-acquireCtx.Done():
			atomic.AddUint64(&p.acquireFailed, 1)
			if acquireCtx.Err() == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, acquireCtx.Err()
		default:
		}

		// Try to get an idle connection
		conn, ok := p.getIdleLocked()
		if ok {
			atomic.AddUint64(&p.acquireSuccess, 1)
			log.Debug("acquired idle connection from pool")
			return conn, nil
		}

		// Try to create a new connection if under limit
		if p.numOpen < p.config.MaxSize {
			p.numOpen++
			p.mu.Unlock()

			conn, err := p.factory(acquireCtx)
			if err != nil {
				p.mu.Lock()
				p.numOpen--
				p.cond.Signal()
				atomic.AddUint64(&p.acquireFailed, 1)
				log.WithError(err).Debug("failed to create new connection")
				return nil, err
			}

			p.mu.Lock()
			atomic.AddUint64(&p.acquireSuccess, 1)
			log.Debug("created new connection")
			return conn, nil
		}

		// Wait for a connection to be released
		log.Debug("waiting for available connection")
		p.waitWithContext(acquireCtx)
	}
}

// getIdleLocked gets an idle connection (caller must hold lock).
// It removes stale connections and checks health if configured.
func (p *Pool) getIdleLocked() (Connection, bool) {
	now := time.Now()
	for len(p.idle) > 0 {
		// Get the most recently used connection (LIFO)
		pc := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		// Check if connection is too old
		if now.Sub(pc.lastUsed) > p.config.MaxIdleTime {
			log.Debug("closing stale connection")
			p.numOpen--
			go pc.conn.Close()
			continue
		}

		// Health check if configured
		if p.config.HealthCheck != nil && !p.config.HealthCheck(pc.conn) {
			log.Debug("closing unhealthy connection")
			atomic.AddUint64(&p.healthFails, 1)
			p.numOpen--
			go pc.conn.Close()
			continue
		}

		return pc.conn, true
	}
	return nil, false
}

// waitWithContext waits for a condition signal or context cancellation.
func (p *Pool) waitWithContext(ctx context.Context) {
	// Start a goroutine to signal on context cancellation
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.cond.Broadcast()
			p.mu.Unlock()
		case <-done:
		}
	}()
	p.cond.Wait()
	close(done)
}

// Release returns a connection to the pool.
// If the pool is full or closed, the connection is closed instead.
func (p *Pool) Release(conn Connection) {
	if conn == nil {
		return
	}

	atomic.AddUint64(&p.releaseCount, 1)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		log.Debug("pool closed, closing connection")
		p.numOpen--
		go conn.Close()
		return
	}

	// Add to idle pool
	pc := &pooledConn{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
	p.idle = append(p.idle, pc)
	p.cond.Signal()
	log.Debug("connection released to pool")
}

// Discard removes a connection from the pool without returning it.
// Use this when a connection is known to be bad.
func (p *Pool) Discard(conn Connection) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	p.numOpen--
	p.cond.Signal()
	p.mu.Unlock()

	log.Debug("discarding bad connection")
	conn.Close()
}

// Close closes the pool and all connections.
func (p *Pool) Close() error {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}

	p.closed = true
	close(p.stopHealth)

	// Close all idle connections
	for _, pc := range p.idle {
		p.numOpen--
		go pc.conn.Close()
	}
	p.idle = nil

	p.cond.Broadcast()
	p.mu.Unlock()

	// Wait for health check goroutine
	<-p.healthDone

	log.Debug("pool closed")
	return nil
}

// healthCheckLoop periodically checks idle connections.
func (p *Pool) healthCheckLoop() {
	defer close(p.healthDone)

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopHealth:
			return
		case <-ticker.C:
			p.runHealthCheck()
		}
	}
}

// runHealthCheck removes unhealthy idle connections.
func (p *Pool) runHealthCheck() {
	if p.config.HealthCheck == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	var toClose []Connection
	healthy := make([]*pooledConn, 0, len(p.idle))
	now := time.Now()

	for _, pc := range p.idle {
		// Check if too old
		if now.Sub(pc.lastUsed) > p.config.MaxIdleTime {
			toClose = append(toClose, pc.conn)
			p.numOpen--
			continue
		}

		// Health check
		if !p.config.HealthCheck(pc.conn) {
			atomic.AddUint64(&p.healthFails, 1)
			toClose = append(toClose, pc.conn)
			p.numOpen--
			continue
		}

		healthy = append(healthy, pc)
	}

	p.idle = healthy

	// Close bad connections outside the lock
	for _, conn := range toClose {
		go conn.Close()
	}

	if len(toClose) > 0 {
		log.WithField("closed", len(toClose)).Debug("health check removed connections")
	}
}

// Stats returns pool statistics.
type Stats struct {
	// MaxSize is the maximum pool size.
	MaxSize int
	// NumOpen is the current number of open connections.
	NumOpen int
	// NumIdle is the current number of idle connections.
	NumIdle int
	// NumInUse is the number of connections currently in use.
	NumInUse int
	// AcquireCount is the total number of acquire attempts.
	AcquireCount uint64
	// AcquireSuccess is the number of successful acquires.
	AcquireSuccess uint64
	// AcquireFailed is the number of failed acquires.
	AcquireFailed uint64
	// ReleaseCount is the number of releases.
	ReleaseCount uint64
	// HealthCheckFails is the number of connections that failed health checks.
	HealthCheckFails uint64
}

// Stats returns current pool statistics.
func (p *Pool) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()

	return Stats{
		MaxSize:          p.config.MaxSize,
		NumOpen:          p.numOpen,
		NumIdle:          len(p.idle),
		NumInUse:         p.numOpen - len(p.idle),
		AcquireCount:     atomic.LoadUint64(&p.acquireCount),
		AcquireSuccess:   atomic.LoadUint64(&p.acquireSuccess),
		AcquireFailed:    atomic.LoadUint64(&p.acquireFailed),
		ReleaseCount:     atomic.LoadUint64(&p.releaseCount),
		HealthCheckFails: atomic.LoadUint64(&p.healthFails),
	}
}
