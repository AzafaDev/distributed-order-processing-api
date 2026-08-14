package product

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
	"github.com/google/uuid"
)

type ProductHandler struct {
	srv      *ProductService
	validate *validator.Validate
	log      *slog.Logger
}

func NewProductHandler(srv *ProductService, log *slog.Logger) *ProductHandler {
	return &ProductHandler{
		srv:      srv,
		validate: validator.New(),
		log:      log,
	}
}

func (h *ProductHandler) RegisterRoutes(r chi.Router) {
	r.Route("/products", func(r chi.Router) {
		r.Get("/", h.ListProducts)
		r.Post("/", h.CreateProduct)
		r.Get("/{id}", h.GetProductByID)
		r.Patch("/{id}", h.UpdateProduct)
		r.Delete("/{id}", h.DeleteProduct)
	})
}

func parseProductID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}

func (h *ProductHandler) ListProducts(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	if pageStr != "" {
		var err error
		page, err = strconv.Atoi(pageStr)
		if err != nil {
			h.log.Error("list products", "error", err)
			httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid page format")
			return
		}
	}

	limit := 10
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			h.log.Error("list products", "error", err)
			httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid limit format")
			return
		}
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

func (h *ProductHandler) GetProductByID(w http.ResponseWriter, r *http.Request) {
	productID, err := parseProductID(r)
	if err != nil {
		h.log.Error("get product by id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid id format")
		return
	}

	existingProduct, err := h.srv.GetProductByID(r.Context(), productID)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			h.log.Error("get product by id", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		h.log.Error("get product by id", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, existingProduct)
}

func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("create product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.Error("create product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid product payload")
		return
	}

	createdProduct, err := h.srv.CreateProduct(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrExistingProductName) {
			h.log.Error("create product", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}
		h.log.Error("create product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, createdProduct)
}

func (h *ProductHandler) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	productID, err := parseProductID(r)
	if err != nil {
		h.log.Error("update product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid id format")
		return
	}

	var req UpdateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("update product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.Error("update product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid product payload")
		return
	}

	updatedProduct, err := h.srv.UpdateProduct(r.Context(), productID, req)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			h.log.Error("update product", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrExistingProductName) {
			h.log.Error("update product", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, err.Error())
			return
		}
		h.log.Error("update product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, updatedProduct)
}

func (h *ProductHandler) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	productID, err := parseProductID(r)
	if err != nil {
		h.log.Error("delete product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid id format")
		return
	}

	if err := h.srv.DeleteProduct(r.Context(), productID); err != nil {
		if errors.Is(err, ErrProductNotFound) {
			h.log.Error("delete product", "error", err)
			httpx.WriteErrorJSON(w, http.StatusNotFound, err.Error())
			return
		}
		h.log.Error("delete product", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
