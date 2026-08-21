package server

import (
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/health"
	"github.com/AzafaDev/distributed-order-processing-api/internal/order"
	"github.com/AzafaDev/distributed-order-processing-api/internal/payment"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/metrics"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	Health  *health.Handler
	User    *user.UserHandler
	Product *product.ProductHandler
	Order   *order.OrderHandler
	Payment *payment.PaymentHandler
}

func NewRouter(h Handler, m *metrics.Metrics) http.Handler {
	r := chi.NewRouter()

	// span name is already set by otelhttp from r.Pattern which is
	// filled by chi, and http.server is fallback for unmatched route.
	r.Use(otelhttp.NewMiddleware("http.server", otelhttp.WithFilter(func(r *http.Request) bool {
		return r.URL.Path != "/livez" && r.URL.Path != "/readyz"
	})))

	r.Use(routeTag)
	r.Use(m.Middleware)

	h.Health.RegisterRoutes(r)

	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			h.User.RegisterRoutes(r)
			h.Product.RegisterRoutes(r)
			h.Order.RegisterRoutes(r)
			h.Payment.RegisterRoutes(r)
		})
	})

	return r
}

func routeTag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		pattern := chi.RouteContext(r.Context()).RoutePattern()
		if pattern == "" {
			return
		}

		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(semconv.HTTPRoute(pattern))
	})
}
