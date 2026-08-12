package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func (h *Handler) ReadyZ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "DB is not ready")
	}

	if err := h.rdb.Ping(r.Context()).Err(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintln(w, "Redis is not ready")
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "DB and redis are ready")
}

func (h *Handler) TestingGraceful(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, "starting 5 seconds to sleep")
	time.Sleep(5 * time.Second)
	fmt.Fprintln(w, "finished to sleep for 5 seconds")
}
