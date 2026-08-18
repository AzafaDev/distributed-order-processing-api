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

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/tracing"
	"github.com/AzafaDev/distributed-order-processing-api/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error config: %v", err)
	}
	log := logger.New(cfg.GoEnv)

	ctx := context.Background()

	tp, err := tracing.New(ctx, cfg, log)
	if err != nil {
		log.Warn("tracing disabled, continuing without it", "error", err)
	}

	app, err := server.BuildApp(ctx, cfg, log)
	if err != nil {
		log.Error("failed to build app", "error", err)
		os.Exit(1)
	}

	serv := http.Server{
		Addr:    ":" + cfg.Port,
		Handler: app.Router,
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

	if tp != nil {
		log.Info("closing tracing open telemetry")
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Error("failed to shutdown tracing open telemetry", "error", err)
		}
	}

	log.Info("closing database gracefully")
	app.DB.Close()

	log.Info("closing redis connection gracefully")
	app.Redis.Close()

	log.Info("graceful shutdown successfully")
}
