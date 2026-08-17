package health

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type DB interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	db  DB
	rdb *redis.Client
	log *slog.Logger
}

func New(db *pgxpool.Pool, rdb *redis.Client, log *slog.Logger) *Handler {
	return &Handler{
		db:  db,
		rdb: rdb,
		log: log,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/livez", h.LiveZ)
	r.Get("/readyz", h.ReadyZ)
}

func (h *Handler) LiveZ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func (h *Handler) ReadyZ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		httpx.WriteErrorJSON(w, http.StatusServiceUnavailable, "DB is not ready")
		return
	}

	redisStatus := "up"
	if err := h.rdb.Ping(r.Context()).Err(); err != nil {
		h.log.Warn("readyz: redis is down (non-fatal, optional dependency)", "error", err)
		redisStatus = "down"
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"db":    "up",
		"redis": redisStatus,
	})
}
