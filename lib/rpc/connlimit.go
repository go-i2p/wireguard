// Package rpc provides connection limiting for the RPC server.
package rpc

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
)

// Connection limit constants
const (
	// DefaultMaxConnections is the default maximum concurrent connections.
	DefaultMaxConnections = 100
)

// Connection limit errors
var (
	// ErrTooManyConnections is returned when the connection limit is reached.
	ErrTooManyConnections = errors.New("too many connections")
)

// ConnectionLimiter manages concurrent connection limits.
// It tracks active connections and rejects new ones when the limit is reached.
type ConnectionLimiter struct {
	maxConns    int32
	activeConns int32
	mu          sync.RWMutex
	onReject    func(addr net.Addr) // optional callback for rejections
}

// NewConnectionLimiter creates a new connection limiter.
// maxConns specifies the maximum number of concurrent connections.
// If maxConns <= 0, DefaultMaxConnections is used.
func NewConnectionLimiter(maxConns int) *ConnectionLimiter {
	if maxConns <= 0 {
		maxConns = DefaultMaxConnections
	}
	return &ConnectionLimiter{
		maxConns: int32(maxConns),
	}
}

// SetOnReject sets a callback function that is called when a connection is rejected.
// The callback receives the address of the rejected connection.
func (cl *ConnectionLimiter) SetOnReject(fn func(addr net.Addr)) {
	cl.mu.Lock()
	cl.onReject = fn
	cl.mu.Unlock()
}

// Acquire attempts to acquire a connection slot.
// Returns true if a slot was acquired, false if the limit is reached.
func (cl *ConnectionLimiter) Acquire() bool {
	for {
		current := atomic.LoadInt32(&cl.activeConns)
		if current >= cl.maxConns {
			return false
		}
		if atomic.CompareAndSwapInt32(&cl.activeConns, current, current+1) {
			return true
		}
	}
}

// Release releases a connection slot.
func (cl *ConnectionLimiter) Release() {
	atomic.AddInt32(&cl.activeConns, -1)
}

// TryAccept attempts to accept a connection, respecting the limit.
// If the limit is reached, it closes the connection and returns nil.
// Otherwise, it returns the connection for handling.
func (cl *ConnectionLimiter) TryAccept(conn net.Conn) net.Conn {
	if !cl.Acquire() {
		cl.mu.RLock()
		onReject := cl.onReject
		cl.mu.RUnlock()

		if onReject != nil {
			onReject(conn.RemoteAddr())
		}
		conn.Close()
		return nil
	}
	return conn
}

// ActiveConnections returns the current number of active connections.
func (cl *ConnectionLimiter) ActiveConnections() int {
	return int(atomic.LoadInt32(&cl.activeConns))
}

// MaxConnections returns the maximum allowed connections.
func (cl *ConnectionLimiter) MaxConnections() int {
	return int(cl.maxConns)
}

// SetMaxConnections updates the maximum connection limit.
// This can be used to dynamically adjust the limit at runtime.
func (cl *ConnectionLimiter) SetMaxConnections(max int) {
	if max <= 0 {
		max = DefaultMaxConnections
	}
	atomic.StoreInt32(&cl.maxConns, int32(max))
}

// LimitedConn wraps a connection to automatically release the slot when closed.
type LimitedConn struct {
	net.Conn
	limiter  *ConnectionLimiter
	released bool
	mu       sync.Mutex
}

// WrapConn wraps a connection with automatic slot release on close.
func (cl *ConnectionLimiter) WrapConn(conn net.Conn) *LimitedConn {
	return &LimitedConn{
		Conn:    conn,
		limiter: cl,
	}
}

// Close closes the connection and releases the connection slot.
func (lc *LimitedConn) Close() error {
	lc.mu.Lock()
	if !lc.released {
		lc.released = true
		lc.limiter.Release()
	}
	lc.mu.Unlock()
	return lc.Conn.Close()
}
