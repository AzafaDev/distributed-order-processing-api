package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/middleware"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/ratelimit"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator"
)

type LoginLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	RecordFailure(ctx context.Context, key string) (int64, error)
	Reset(ctx context.Context, key string) error
}

type UserHandler struct {
	srv      *UserService
	validate *validator.Validate
	log      *slog.Logger
	jwt      *auth.JWTManager
	rl       LoginLimiter
}

func NewUserHandler(srv *UserService, log *slog.Logger, jwt *auth.JWTManager, rl LoginLimiter) *UserHandler {
	return &UserHandler{
		srv:      srv,
		validate: validator.New(),
		log:      log,
		jwt:      jwt,
		rl:       rl,
	}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.With(middleware.LoginRateLimiter(h.rl, h.log)).Post("/login", h.Login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(h.jwt, h.log))
			r.Get("/me", h.Me)
		})
	})
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.WarnContext(r.Context(), "register", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.WarnContext(r.Context(), "register", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid email or password format")
		return
	}

	createdUser, err := h.srv.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrEmailRegistered) {
			h.log.WarnContext(r.Context(), "register", "error", err)
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
		h.log.WarnContext(r.Context(), "login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := h.validate.Struct(req); err != nil {
		h.log.WarnContext(r.Context(), "login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusBadRequest, "invalid email or password")
		return
	}

	rlKey := ratelimit.LoginKey(httpx.ClientIP(r))

	signedToken, err := h.srv.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrLoginGeneric) {
			h.recordLoginFailure(r.Context(), rlKey)
			h.log.WarnContext(r.Context(), "login", "error", err)
			httpx.WriteErrorJSON(w, http.StatusUnauthorized, err.Error())
			return
		}

		h.log.ErrorContext(r.Context(), "login", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}

	if err := h.rl.Reset(r.Context(), rlKey); err != nil {
		h.log.ErrorContext(r.Context(), "login: reset rate limit counter", "error", err)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"token": signedToken})
}

func (h *UserHandler) recordLoginFailure(ctx context.Context, key string) {
	count, err := h.rl.RecordFailure(ctx, key)
	if err != nil {
		h.log.ErrorContext(ctx, "login: record failure", "error", err)
		return
	}
	h.log.InfoContext(ctx, "login: failed attempt recorded", "key", key, "count", count)
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDClaims(r.Context())
	if err != nil {
		h.log.ErrorContext(r.Context(), "Me", "error", err)
		httpx.WriteErrorJSON(w, http.StatusInternalServerError, "something went wrong")
		return
	}
	fmt.Fprintln(w, userID)
}
