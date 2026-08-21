// Package metrics exposes application metrics in Prometheus format.
//
// It deliberately uses prometheus/client_golang directly instead of the
// OpenTelemetry metrics SDK: there is a single backend here, and the OTel
// naming translation would make the metric names diverge from every PromQL
// example and Grafana dashboard in the wild.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Label values are kept low-cardinality on purpose. Route is always a chi
// route pattern (never a raw URL), and anything that did not match a route
// collapses into routeUnmatched so that scanners hitting random paths cannot
// grow the time series count without bound.
const routeUnmatched = "unmatched"

type Metrics struct {
	registry *prometheus.Registry

	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	errorsTotal     *prometheus.CounterVec
	inFlight        prometheus.Gauge
}

func New() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		registry: reg,

		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests served, by route pattern, method and status code.",
		}, []string{"route", "method", "status"}),

		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "HTTP request latency in seconds, by route pattern and method.",
			// The sub-millisecond buckets matter: cached reads land well under
			// 1ms, and without them every percentile is just interpolation
			// inside a single bucket. The tail buckets exist so a degraded P99
			// stays visible instead of saturating the top bucket.
			Buckets: []float64{
				0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05,
				0.1, 0.25, 0.5, 1, 2.5, 5, 10,
			},
		}, []string{"route", "method"}),

		errorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total number of HTTP requests that failed, split into client (4xx) and server (5xx) errors.",
		}, []string{"route", "method", "class"}),

		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being served.",
		}),
	}

	reg.MustRegister(
		m.requestsTotal,
		m.requestDuration,
		m.errorsTotal,
		m.inFlight,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return m
}

// Handler serves the Prometheus exposition endpoint. It is mounted on its own
// listener (see cmd/api) so scrapes never travel through the public router's
// tracing, auth or rate limiting middleware.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		Registry: m.registry,
	})
}
