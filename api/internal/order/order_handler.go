package order

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/idempotency"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/middleware"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
	"github.com/google/uuid"
)

type OrderHandler struct {
	s   *OrderService
	log *slog.Logger
	v   *validator.Validate
	jw  *auth.JWTManager
	is  *idempotency.IdempotencyService
}

func NewOrderHandler(s *OrderService, log *slog.Logger, jw *auth.JWTManager, is *idempotency.IdempotencyService) *OrderHandler {
	return &OrderHandler{
		s:   s,
		log: log,
		v:   validator.New(),
		jw:  jw,
		is:  is,
	}
}

// RegisterRoutes mounts the order routes onto an existing /orders router.
//
// The router is shared with the payment module rather than each module
// mounting its own /orders subtree: two sibling subtrees ("/orders" and
// "/orders/{id}") would shadow each other in chi's trie, and whichever one
// owns "/orders/{id}/..." silently 404s the other's routes.
func (h *OrderHandler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(h.jw, h.log))
		r.Get("/", h.GetOrders)
		r.Get("/{id}", h.GetOrderByID)
		r.Post("/{id}/cancel", h.CancelOrder)
		r.With(middleware.IdempotencyMiddleware(h.is, h.log)).Post("/", h.CreateOrder)
	})
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.ErrorContext(r.Context(), "create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.v.Struct(req); err != nil {
		h.log.ErrorContext(r.Context(), "create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid request")
		return
	}

	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idempotencyVal, err := middleware.GetIdempotencyValueFromContext(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	createdOrder, createdOrderItems, createdPayment, err := h.s.CreateOrder(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, product.ErrProductNotFound) {
			h.log.ErrorContext(r.Context(), "create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		if errors.Is(err, product.ErrInsufficientStock) {
			h.log.ErrorContext(r.Context(), "create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}

		if errors.Is(err, ErrOrderNotFound) {
			h.log.ErrorContext(r.Context(), "create order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		h.log.ErrorContext(r.Context(), "create order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	resp := CreateOrderResponse{
		Order:   *createdOrder,
		Items:   createdOrderItems,
		Payment: *createdPayment,
	}

	envelope := httpx.ResponseJson{Success: true, Data: resp}
	respJSON, err := json.Marshal(envelope)
	if err != nil {
		h.log.ErrorContext(r.Context(), "create order: marshal response for idempotency", "error", err)
	} else if err := h.is.SaveResponse(r.Context(), idempotencyVal, userID, respJSON); err != nil {
		h.log.ErrorContext(r.Context(), "create order: save idempotency response", "error", err)
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "get orders", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orders, err := h.s.GetOrders(r.Context(), userID)
	if err != nil {
		h.log.ErrorContext(r.Context(), "get orders", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, orders)
}

func (h *OrderHandler) GetOrderByID(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.log.ErrorContext(r.Context(), "get order by id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid id format")
		return
	}

	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "get order by id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	order, err := h.s.GetOrderByID(r.Context(), userID, orderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			h.log.ErrorContext(r.Context(), "get order by id", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		h.log.ErrorContext(r.Context(), "get order by id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, order)
}

func (h *OrderHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.log.ErrorContext(r.Context(), "cancel order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid id format")
		return
	}

	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "cancel order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cancelledOrder, err := h.s.CancelOrder(r.Context(), userID, orderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			h.log.ErrorContext(r.Context(), "cancel order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		if errors.Is(err, ErrOrderNotPending) {
			h.log.ErrorContext(r.Context(), "cancel order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}

		if errors.Is(err, ErrOrderItemsUnvailable) {
			h.log.ErrorContext(r.Context(), "cancel order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		// Restoring stock cannot fail on stock; it can only fail if the
		// product row is gone.
		if errors.Is(err, product.ErrProductNotFound) {
			h.log.ErrorContext(r.Context(), "cancel order", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		h.log.ErrorContext(r.Context(), "cancel order", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, cancelledOrder)

}
