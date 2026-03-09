package main

import (
	"log"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	// Import metrics package so promauto registrations take effect.
	_ "github.com/trogers1052/context-service/internal/metrics"
)

func startMetricsServer() {
	port := os.Getenv("METRICS_PORT")
	if port == "" {
		port = "9092"
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":"+port, metricsMux); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()
	log.Printf("Metrics server listening on :%s/metrics", port)
}
