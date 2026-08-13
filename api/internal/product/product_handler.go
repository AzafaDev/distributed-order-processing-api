package product

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

type ProductHandler struct {
	srv *ProductService
	log *slog.Logger
}

func NewProductHandler(srv *ProductService, log *slog.Logger) *ProductHandler {
	return &ProductHandler{
		srv: srv,
		log: log,
	}
}

func (h *ProductHandler) RegisterRoutes(r chi.Router) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", h.ListProducts)
	})
}

func (h *ProductHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page, err := strconv.Atoi(pageStr)
	if err != nil {
		h.log.Error("list products", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid page format")
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		h.log.Error("list products", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid limit format")
		return
	}

	productsList, err := h.srv.ListProducts(r.Context(), page, limit)
	if err != nil {
		if errors.Is(err, ErrNoProduct) {
			h.log.Info("list products", "products", productsList)
			httpx.WriteJSON(w, http.StatusOK, []Product{})
			return
		}
		h.log.Error("list products", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, productsList)
}
