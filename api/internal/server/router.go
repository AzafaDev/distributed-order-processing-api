package server

import (
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/health"
	"github.com/AzafaDev/distributed-order-processing-api/internal/order"
	"github.com/AzafaDev/distributed-order-processing-api/internal/payment"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Health  *health.Handler
	User    *user.UserHandler
	Product *product.ProductHandler
	Order   *order.OrderHandler
	Payment *payment.PaymentHandler
}

func NewRouter(h Handler) http.Handler {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			h.User.RegisterRoutes(r)
			h.Health.RegisterRoutes(r)
			h.Product.RegisterRoutes(r)
			h.Order.RegisterRoutes(r)
			h.Payment.RegisterRoutes(r)
		})
	})

	return r
}
