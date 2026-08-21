package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/health"
	"github.com/AzafaDev/distributed-order-processing-api/internal/order"
	"github.com/AzafaDev/distributed-order-processing-api/internal/payment"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/metrics"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// Handlers are never entered by these tests — only the router's own resolution
// is under test — so the services behind them can be nil.
func newTestRouter() http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwt := auth.NewJWTManager("router-test-secret", time.Hour)

	return NewRouter(Handler{
		Health:  health.New(nil, nil, log),
		User:    user.NewUserHandler(nil, log, jwt, nil),
		Product: product.NewProductHandler(nil, log),
		Order:   order.NewOrderHandler(nil, log, jwt, nil),
		Payment: payment.NewPaymentHandler(nil, jwt, log),
	}, metrics.New())
}

// The order and payment modules both serve paths under /orders/{id}. While each
// mounted its own subtree, chi resolved /orders/{id}/... inside whichever
// module owned that subtree, so POST /orders/{id}/cancel was dead in
// production — it answered 404 while the payment routes beside it worked.
//
// This has to use Match rather than sending requests: a request to a shadowed
// path still runs the shadowing subrouter's auth middleware, which answers 401
// before the 404 is ever reached, so an unauthenticated request cannot tell a
// live route from a dead one.
func TestRouterResolvesEveryDocumentedRoute(t *testing.T) {
	router := newTestRouter().(chi.Routes)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/livez"},
		{http.MethodGet, "/readyz"},
		{http.MethodPost, "/api/users/register"},
		{http.MethodPost, "/api/users/login"},
		{http.MethodGet, "/api/users/me"},
		{http.MethodGet, "/api/products/"},
		{http.MethodPost, "/api/products/"},
		{http.MethodGet, "/api/products/11111111-1111-1111-1111-111111111111"},
		{http.MethodPatch, "/api/products/11111111-1111-1111-1111-111111111111"},
		{http.MethodDelete, "/api/products/11111111-1111-1111-1111-111111111111"},
		{http.MethodGet, "/api/orders/"},
		{http.MethodPost, "/api/orders/"},
		{http.MethodGet, "/api/orders/11111111-1111-1111-1111-111111111111"},
		{http.MethodPost, "/api/orders/11111111-1111-1111-1111-111111111111/cancel"},
		{http.MethodPost, "/api/orders/11111111-1111-1111-1111-111111111111/pay"},
		{http.MethodGet, "/api/orders/11111111-1111-1111-1111-111111111111/payment"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			require.True(t, router.Match(chi.NewRouteContext(), route.method, route.path),
				"route does not resolve; it would answer 404 in production")
		})
	}
}

func TestRouterServesLivenessProbe(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	require.Equal(t, http.StatusOK, rec.Code)
}
