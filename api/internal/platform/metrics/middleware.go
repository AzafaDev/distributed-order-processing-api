package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// Middleware records request count, latency and errors for every request.
//
// It must run after chi has had a chance to match the request, so the route
// pattern is read once the inner handler returns.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbe(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		m.inFlight.Inc()
		defer m.inFlight.Dec()

		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)

		elapsed := time.Since(start).Seconds()
		route := routePattern(r)
		method := r.Method
		status := ww.Status()
		if status == 0 {
			// Handler returned without ever writing a header.
			status = http.StatusOK
		}

		m.requestsTotal.WithLabelValues(route, method, strconv.Itoa(status)).Inc()
		m.requestDuration.WithLabelValues(route, method).Observe(elapsed)

		switch {
		case status >= 500:
			m.errorsTotal.WithLabelValues(route, method, "server").Inc()
		case status >= 400:
			m.errorsTotal.WithLabelValues(route, method, "client").Inc()
		}
	})
}

// routePattern returns the chi route pattern for the request, never the raw
// URL. Unmatched requests (404s, including scanner traffic hitting random
// paths) all collapse into a single series.
func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return routeUnmatched
	}

	pattern := rctx.RoutePattern()
	if pattern == "" {
		return routeUnmatched
	}

	return pattern
}

func isProbe(path string) bool {
	return path == "/livez" || path == "/readyz"
}
