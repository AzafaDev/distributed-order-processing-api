package health

import (
	"context"
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
}

func New(db *pgxpool.Pool, rdb *redis.Client) *Handler {
	return &Handler{
		db:  db,
		rdb: rdb,
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/livez", h.LiveZ)
	r.Get("/readyz", h.ReadyZ)
	r.Get("/sleep", h.TestingGraceful)
}

func (h *Handler) LiveZ(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (h *Handler) ReadyZ(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		httpx.WriteErrorJSON(w, http.StatusServiceUnavailable, "DB is not ready")
		return
	}

	if err := h.rdb.Ping(r.Context()).Err(); err != nil {
		httpx.WriteErrorJSON(w, http.StatusServiceUnavailable, "Redis is not ready")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "DB and redis are ready"})
}

func (h *Handler) TestingGraceful(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "starting 5 seconds to sleep"})
	time.Sleep(5 * time.Second)
}
