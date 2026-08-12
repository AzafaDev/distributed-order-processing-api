package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	fmt.Println("server is running at port: 8080")
	if err := http.ListenAndServe(":8080", mux); err != nil && err != http.ErrServerClosed {
		log.Fatalf("failed to start server: %v", err)
	}
}
