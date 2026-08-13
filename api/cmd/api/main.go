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
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/database"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
	"github.com/AzafaDev/distributed-order-processing-api/internal/server"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user/sqlc"
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

	jwtManager := auth.NewJWTManager(cfg.JwtSecret, cfg.JwtExpiry)

	healthHandler := health.New(dbPool.Pool, rdb)

	queries := sqlc.New(dbPool.Pool)

	userRepository := user.NewUserRepository(queries)
	userService := user.NewUserService(userRepository, jwtManager)
	userHandler := user.NewUserHandler(userService, log)

	router := server.NewRouter(server.Handler{
		User:   userHandler,
		Health: healthHandler,
	})

	serv := http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
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
