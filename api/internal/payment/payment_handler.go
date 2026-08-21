package payment

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type PaymentHandler struct {
	serv *PaymentService
	jwt  *auth.JWTManager
	log  *slog.Logger
}

func NewPaymentHandler(serv *PaymentService, jwt *auth.JWTManager, log *slog.Logger) *PaymentHandler {
	return &PaymentHandler{
		serv: serv,
		jwt:  jwt,
		log:  log,
	}
}

// RegisterRoutes mounts the payment routes onto the shared /orders router.
// See OrderHandler.RegisterRoutes for why the router is shared.
func (h *PaymentHandler) RegisterRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(h.jwt, h.log))

		r.Post("/{id}/pay", h.Pay)
		r.Get("/{id}/payment", h.GetPaymentByOrderID)
	})
}

func (h *PaymentHandler) Pay(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "pay", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.log.ErrorContext(r.Context(), "pay", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "id format is invalid")
		return
	}

	payment, order, err := h.serv.Pay(r.Context(), userID, orderID)
	if err != nil {
		if errors.Is(err, ErrNoOrderFound) {
			h.log.ErrorContext(r.Context(), "pay", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		if errors.Is(err, ErrNoPaymentFound) {
			h.log.ErrorContext(r.Context(), "pay", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}

		if errors.Is(err, ErrOrderNotPayable) {
			h.log.ErrorContext(r.Context(), "pay", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}

		h.log.ErrorContext(r.Context(), "pay", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, PayResponse{
		Payment: *payment,
		Order:   *order,
	})
}

func (h *PaymentHandler) GetPaymentByOrderID(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "get payment by order id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderIDStr := chi.URLParam(r, "id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		h.log.ErrorContext(r.Context(), "get payment by order id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "id format is invalid")
		return
	}

	payment, err := h.serv.GetPaymentByOrderID(r.Context(), userID, orderID)
	if err != nil {
		if errors.Is(err, ErrNoPaymentFound) {
			h.log.ErrorContext(r.Context(), "get payment by order id", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		h.log.ErrorContext(r.Context(), "get payment by order id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, payment)
}
