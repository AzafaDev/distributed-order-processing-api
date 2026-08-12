package main

import (
	"log"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error config: %v", err)
	}

	log := logger.New(cfg.GoEnv)

	mux := http.NewServeMux()

	log.Info("server is started", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil && err != http.ErrServerClosed {
		log.Error("failed to start server", "error", err)
	}
}
