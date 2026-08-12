package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/database"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error config: %v", err)
	}
	log := logger.New(cfg.GoEnv)

	ctx := context.Background()

	dbPool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to connect DB", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/readyz", readyzHandler(dbPool))

	log.Info("server is started", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil && err != http.ErrServerClosed {
		log.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func readyzHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "database is not ready")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "database is ready")
	}
}
