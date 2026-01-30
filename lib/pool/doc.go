// Package pool provides a generic connection pool implementation for managing
// reusable connections to external services.
//
// The pool supports:
//   - Configurable maximum pool size
//   - Connection idle timeout
//   - Health checking for pooled connections
//   - Metrics for pool utilization
//   - Context-aware acquisition with timeout support
//
// # Basic Usage
//
//	factory := func(ctx context.Context) (pool.Connection, error) {
//	    return net.Dial("tcp", "localhost:8080")
//	}
//
//	cfg := pool.DefaultConfig()
//	cfg.MaxSize = 10
//	cfg.MaxIdleTime = 5 * time.Minute
//
//	p := pool.New(factory, cfg)
//	defer p.Close()
//
//	conn, err := p.Acquire(ctx)
//	if err != nil {
//	    return err
//	}
//	defer p.Release(conn)
//
//	// Use connection...
//
// # Health Checking
//
// The pool supports health checking to remove broken connections:
//
//	cfg.HealthCheck = func(conn pool.Connection) bool {
//	    // Try a ping operation
//	    return conn.(*MyConn).Ping() == nil
//	}
//
// # Metrics
//
// Pool utilization metrics are automatically registered with the metrics package:
//   - i2plan_pool_connections_max: Maximum pool size
//   - i2plan_pool_connections_open: Current open connections
//   - i2plan_pool_connections_idle: Current idle connections
//   - i2plan_pool_connections_in_use: Connections currently in use
//   - i2plan_pool_acquire_total: Total acquire attempts
//   - i2plan_pool_acquire_success_total: Successful acquires
//   - i2plan_pool_acquire_failed_total: Failed acquires
//   - i2plan_pool_release_total: Total releases
//   - i2plan_pool_healthcheck_fails_total: Health check failures
package pool
