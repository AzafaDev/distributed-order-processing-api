package user

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
)

type UserHandler struct {
	srv      *UserService
	validate *validator.Validate
}

func NewUserHandler(srv *UserService) *UserHandler {
	return &UserHandler{
		srv:      srv,
		validate: validator.New(),
	}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/register", h.Register)
	})
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid email or password format")
		return
	}

	createdUser, err := h.srv.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrEmailRegistered) {
			httpx.WriteErrorJSON(w, http.StatusConflict, ErrEmailRegistered.Error())
			return
		}
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, createdUser)
}
