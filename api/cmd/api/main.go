package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/health"
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

	healthHandler := health.New(dbPool.Pool, rdb)

	mux.HandleFunc("/readyz", healthHandler.ReadyZ)
	mux.HandleFunc("/sleep", healthHandler.TestingGraceful)
	mux.HandleFunc("/livez", healthHandler.LiveZ)

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
