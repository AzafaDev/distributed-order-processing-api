package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
)

type UserHandler struct {
	srv      *UserService
	validate *validator.Validate
	log      *slog.Logger
	jwt      *auth.JWTManager
}

func NewUserHandler(srv *UserService, log *slog.Logger, jwt *auth.JWTManager) *UserHandler {
	return &UserHandler{
		srv:      srv,
		validate: validator.New(),
		log:      log,
		jwt:      jwt,
	}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(h.jwt, h.log))
			r.Get("/me", h.Me)
		})
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

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.Error("Me", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}
	fmt.Fprintln(w, userID)
}
