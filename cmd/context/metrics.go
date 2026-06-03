package main

import (
	"log"

	"github.com/trogers1052/trading-go-commons/env"
	"github.com/trogers1052/trading-go-commons/httpserver"

	// Import metrics package so promauto registrations take effect.
	_ "github.com/trogers1052/context-service/internal/metrics"
)

func startMetricsServer() {
	port := env.String("METRICS_PORT", "9092")
	srv := httpserver.NewMetricsServer(":" + port)
	errCh := srv.Start()
	go func() {
		if err := <-errCh; err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	log.Printf("Metrics server listening on :%s/metrics", port)
}
