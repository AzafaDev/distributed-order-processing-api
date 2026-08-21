package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func newTestRouter(m *Metrics) http.Handler {
	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Get("/api/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Get("/api/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {})
	return r
}

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	return rec.Body.String()
}

func do(router http.Handler, path string) {
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
}

// The whole point of the middleware is that the route label stays bounded, so
// two different order IDs must land on one series carrying the chi pattern.
func TestMiddlewareLabelsRoutePatternNotRawURL(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	do(router, "/api/orders/1111")
	do(router, "/api/orders/2222")

	body := scrape(t, m)

	require.Contains(t, body, `http_requests_total{method="GET",route="/api/orders/{id}",status="200"} 2`)
	require.NotContains(t, body, "1111")
	require.NotContains(t, body, "2222")
}

// Unmatched paths are what a scanner produces; they must all collapse into a
// single series instead of one per URL.
func TestMiddlewareCollapsesUnmatchedRoutes(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	do(router, "/nope/a")
	do(router, "/nope/b")

	body := scrape(t, m)

	require.Contains(t, body, `http_requests_total{method="GET",route="unmatched",status="404"} 2`)
	require.NotContains(t, body, "/nope/")
}

func TestMiddlewareSplitsErrorsByClass(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	do(router, "/api/boom")
	do(router, "/nope")

	body := scrape(t, m)

	require.Contains(t, body, `http_request_errors_total{class="server",method="GET",route="/api/boom"} 1`)
	require.Contains(t, body, `http_request_errors_total{class="client",method="GET",route="unmatched"} 1`)
}

func TestMiddlewareSkipsHealthProbes(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	do(router, "/livez")

	body := scrape(t, m)

	for _, line := range strings.Split(body, "\n") {
		require.NotContains(t, line, `route="/livez"`)
	}
}

func TestMiddlewareRecordsLatencyHistogram(t *testing.T) {
	m := New()
	router := newTestRouter(m)

	do(router, "/api/orders/1")

	body := scrape(t, m)

	require.Contains(t, body, `http_request_duration_seconds_count{method="GET",route="/api/orders/{id}"} 1`)
	require.Contains(t, body, `http_request_duration_seconds_bucket{method="GET",route="/api/orders/{id}",le="10"}`)
}
