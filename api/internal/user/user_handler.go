package user

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
)

type UserHandler struct {
	srv      *UserService
	validate *validator.Validate
	log      *slog.Logger
}

func NewUserHandler(srv *UserService, log *slog.Logger) *UserHandler {
	return &UserHandler{
		srv:      srv,
		validate: validator.New(),
		log:      log,
	}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
	})
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("register", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.Error("register", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid email or password format")
		return
	}

	createdUser, err := h.srv.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrEmailRegistered) {
			h.log.Error("register", "error", err)
			httpx.WriteErrorJSON(w, http.StatusConflict, ErrEmailRegistered.Error())
			return
		}
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, createdUser)
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.Error("login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid email or password")
		return
	}

	signedToken, err := h.srv.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrLoginGeneric) {
			h.log.Error("login", "error", err)
			httpx.WriteErrorJSON(w, http.StatusBadRequest, err.Error())
			return
		}

		h.log.Error("login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"token": signedToken})
}
