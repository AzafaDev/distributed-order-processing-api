package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/database"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
	"github.com/redis/go-redis/v9"
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

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost + ":" + cfg.RedisPort,
		Password: "",
		Protocol: 3,
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/readyz", readyzHandler(dbPool))
	mux.HandleFunc("/sleep", testingGraceful(log))

	serv := http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	serverErr := make(chan error, 1)

	go func() {
		log.Info("server is starting", "port", cfg.Port)
		err := serv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		log.Error("failed to start server", "error", err)
		return
	case <-signalCtx.Done():
		log.Error("received signal to shutdown server gracefully")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Info("server is shutting down")

	log.Info("shutdown server gracefully")
	if err := serv.Shutdown(shutdownCtx); err != nil {
		log.Error("failed to shutdown server gracefully", "error", err)
	}

	log.Info("closing database gracefully")
	dbPool.Close()

	log.Info("closing redis connection gracefully")
	rdb.Close()

	log.Info("graceful shutdown successfully")
}

func readyzHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "database is not ready")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "database is ready")
	}
}

func testingGraceful(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "starting to sleep in 5 seconds")
		time.Sleep(5 * time.Second)
		fmt.Fprintln(w, "finished to sleep 5 seconds")
	}
}
