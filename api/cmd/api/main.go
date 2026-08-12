package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("error config: %v", err)
	}

	mux := http.NewServeMux()
	
	fmt.Printf("server is running at port: %s\n", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to start server: %v", err)
	}
}
