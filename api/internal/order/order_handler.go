package order

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/middleware"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
)

type OrderHandler struct {
	s   *OrderService
	log *slog.Logger
	v   *validator.Validate
	jw  *auth.JWTManager
}

func NewOrderHandler(s *OrderService, log *slog.Logger, jw *auth.JWTManager) *OrderHandler {
	return &OrderHandler{
		s:   s,
		log: log,
		v:   validator.New(),
		jw:  jw,
	}
}

func (h *OrderHandler) RegisterRoutes(r chi.Router) {
	r.Route("/orders", func(r chi.Router) {
		r.Use(middleware.Auth(h.jw, h.log))
		r.Post("/", h.CreateOrder)
	})
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.v.Struct(req); err != nil {
		h.log.Error("create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.Error("create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	createdOrder, createdOrderItems, createdPayment, err := h.s.CreateOrder(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, product.ErrProductNotFound) {
			h.log.Error("create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		if errors.Is(err, product.ErrInsufficientStock) {
			h.log.Error("create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}

		if errors.Is(err, ErrOrderNotFound) {
			h.log.Error("create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		h.log.Error("create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, CreateOrderResponse{
		Order:   *createdOrder,
		Items:   createdOrderItems,
		Payment: *createdPayment,
	})
}
