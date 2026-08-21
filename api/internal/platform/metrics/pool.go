package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolCollector reports pgx connection pool statistics. It is a custom
// collector rather than a set of GaugeFuncs so that a single pool.Stat()
// snapshot backs every metric in one scrape, keeping the numbers consistent
// with each other.
type poolCollector struct {
	pool *pgxpool.Pool

	acquired      *prometheus.Desc
	idle          *prometheus.Desc
	total         *prometheus.Desc
	max           *prometheus.Desc
	constructing  *prometheus.Desc
	acquireCount  *prometheus.Desc
	emptyAcquires *prometheus.Desc
}

func newPoolCollector(pool *pgxpool.Pool) *poolCollector {
	return &poolCollector{
		pool: pool,
		acquired: prometheus.NewDesc(
			"db_pool_acquired_connections",
			"Number of connections currently checked out of the pgx pool.",
			nil, nil),
		idle: prometheus.NewDesc(
			"db_pool_idle_connections",
			"Number of idle connections in the pgx pool.",
			nil, nil),
		total: prometheus.NewDesc(
			"db_pool_total_connections",
			"Total number of connections currently held by the pgx pool.",
			nil, nil),
		max: prometheus.NewDesc(
			"db_pool_max_connections",
			"Configured maximum size of the pgx pool.",
			nil, nil),
		constructing: prometheus.NewDesc(
			"db_pool_constructing_connections",
			"Number of connections the pgx pool is currently establishing.",
			nil, nil),
		acquireCount: prometheus.NewDesc(
			"db_pool_acquire_total",
			"Cumulative number of successful connection acquisitions from the pgx pool.",
			nil, nil),
		emptyAcquires: prometheus.NewDesc(
			"db_pool_empty_acquire_total",
			"Cumulative number of acquisitions that had to wait because the pool was empty.",
			nil, nil),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.max
	ch <- c.constructing
	ch <- c.acquireCount
	ch <- c.emptyAcquires
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.pool.Stat()

	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	gauge(c.acquired, float64(stat.AcquiredConns()))
	gauge(c.idle, float64(stat.IdleConns()))
	gauge(c.total, float64(stat.TotalConns()))
	gauge(c.max, float64(stat.MaxConns()))
	gauge(c.constructing, float64(stat.ConstructingConns()))
	counter(c.acquireCount, float64(stat.AcquireCount()))
	counter(c.emptyAcquires, float64(stat.EmptyAcquireCount()))
}

// RegisterPool exposes the pgx pool statistics on this registry.
func (m *Metrics) RegisterPool(pool *pgxpool.Pool) error {
	return m.registry.Register(newPoolCollector(pool))
}
