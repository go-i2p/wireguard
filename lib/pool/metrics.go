package pool

import "github.com/go-i2p/wireguard/lib/metrics"

// Pool utilization metrics
var (
	// PoolConnectionsTotal is the maximum pool size.
	PoolConnectionsTotal = metrics.NewGauge(
		"i2plan_pool_connections_max",
		"Maximum number of connections in the pool",
	)
	// PoolConnectionsOpen is the current number of open connections.
	PoolConnectionsOpen = metrics.NewGauge(
		"i2plan_pool_connections_open",
		"Current number of open connections",
	)
	// PoolConnectionsIdle is the current number of idle connections.
	PoolConnectionsIdle = metrics.NewGauge(
		"i2plan_pool_connections_idle",
		"Current number of idle connections in the pool",
	)
	// PoolConnectionsInUse is the number of connections currently in use.
	PoolConnectionsInUse = metrics.NewGauge(
		"i2plan_pool_connections_in_use",
		"Number of connections currently in use",
	)
	// PoolAcquireTotal is the total number of acquire attempts.
	PoolAcquireTotal = metrics.NewCounter(
		"i2plan_pool_acquire_total",
		"Total number of connection acquire attempts",
	)
	// PoolAcquireSuccessTotal is the number of successful acquires.
	PoolAcquireSuccessTotal = metrics.NewCounter(
		"i2plan_pool_acquire_success_total",
		"Total number of successful connection acquires",
	)
	// PoolAcquireFailedTotal is the number of failed acquires.
	PoolAcquireFailedTotal = metrics.NewCounter(
		"i2plan_pool_acquire_failed_total",
		"Total number of failed connection acquires",
	)
	// PoolReleaseTotal is the number of releases.
	PoolReleaseTotal = metrics.NewCounter(
		"i2plan_pool_release_total",
		"Total number of connection releases",
	)
	// PoolHealthCheckFailsTotal is the number of health check failures.
	PoolHealthCheckFailsTotal = metrics.NewCounter(
		"i2plan_pool_healthcheck_fails_total",
		"Total number of connections that failed health checks",
	)
	// PoolAcquireLatency tracks time spent acquiring connections.
	PoolAcquireLatency = metrics.NewHistogram(
		"i2plan_pool_acquire_duration_seconds",
		"Time spent acquiring a connection from the pool",
		metrics.DefaultLatencyBuckets,
	)
)

// UpdateMetrics updates the pool metrics from Stats.
func UpdateMetrics(stats Stats) {
	PoolConnectionsTotal.Set(int64(stats.MaxSize))
	PoolConnectionsOpen.Set(int64(stats.NumOpen))
	PoolConnectionsIdle.Set(int64(stats.NumIdle))
	PoolConnectionsInUse.Set(int64(stats.NumInUse))
}
