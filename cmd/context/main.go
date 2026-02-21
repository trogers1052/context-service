package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/service"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg := config.Load()

	// Create service
	svc := service.NewContextService(cfg)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize service
	if err := svc.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize service: %v", err)
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start service in goroutine
	go func() {
		if err := svc.Start(ctx); err != nil {
			log.Printf("Service error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	log.Printf("Received signal: %v", sig)

	// Graceful shutdown
	cancel()
	svc.Stop()
}
