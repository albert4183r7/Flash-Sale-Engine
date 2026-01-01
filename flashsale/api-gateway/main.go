package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flashsale/api-gateway/config"
	"github.com/flashsale/api-gateway/router"
)

// Version is set at build time via -ldflags
var version = "dev"

func main() {
	log.Printf("Starting API Gateway (version: %s)...", version)
	cfg := config.Load()

	r := router.Setup(cfg)

	// Create HTTP server with production timeouts
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("API Gateway listening on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutdown signal received, draining connections...")

	// Create deadline for graceful shutdown
	// Cloud Run/GKE sends SIGTERM, then SIGKILL after termination grace period
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Shutdown gracefully - stops accepting new requests, waits for existing to complete
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("API Gateway shutdown complete")
}