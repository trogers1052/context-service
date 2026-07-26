package main

import (
	"context"
	"log"
	"time"

	"github.com/trogers1052/trading-go-commons/env"
	"github.com/trogers1052/trading-go-commons/httpserver"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/health"
	"github.com/trogers1052/context-service/internal/service"
)

// livenessMaxAge is generous enough to span the longest normal market closure
// (a holiday long-weekend) while still catching a consumer that has been dead
// for days — the failure that hid for 40 days behind an always-200 /health.
const livenessMaxAge = 5 * 24 * time.Hour

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Liveness probe: the consumer beats it on every message it processes, so
	// /health goes 503 if the consume loop dies while quotes are flowing.
	probe := health.NewProbe(livenessMaxAge)

	healthPort := env.String("HEALTH_PORT", "8080")
	healthServer := health.NewServer(":"+healthPort, probe)
	healthErrCh := healthServer.Start()
	go func() {
		if err := <-healthErrCh; err != nil {
			log.Printf("Health server error: %v", err)
		}
	}()
	log.Printf("Health endpoint (liveness): http://localhost:%s/health", healthPort)

	// Metrics endpoint — Prometheus scrape target
	startMetricsServer()

	// Load configuration
	cfg := config.Load()

	// Create service
	svc := service.NewContextService(cfg, probe)

	// Context cancelled on SIGINT/SIGTERM.
	signalCtx, stop := httpserver.SignalContext()
	defer stop()

	// Derive a cancellable context so a service error can also trigger shutdown.
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	// Initialize service
	if err := svc.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize service: %v", err)
	}

	// Start service in goroutine
	go func() {
		if err := svc.Start(ctx); err != nil {
			log.Printf("Service error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal (or service error).
	<-ctx.Done()
	log.Printf("Shutting down")

	// Graceful shutdown (httpserver applies a 5s default timeout).
	cancel()
	if err := healthServer.Shutdown(context.Background()); err != nil {
		log.Printf("Health server shutdown error: %v", err)
	}
	svc.Stop()
}
