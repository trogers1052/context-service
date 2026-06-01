package main

import (
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// pollMetrics retries a GET against the metrics endpoint until it succeeds or
// the deadline passes (the server starts asynchronously in a goroutine).
func pollMetrics(url string, deadline time.Duration) (*http.Response, error) {
	end := time.Now().Add(deadline)
	var lastErr error
	for time.Now().Before(end) {
		resp, err := http.Get(url)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	return nil, lastErr
}

func TestStartMetricsServer_ServesMetricsOnConfiguredPort(t *testing.T) {
	// Use an uncommon high port to avoid clashing with a real metrics server.
	os.Setenv("METRICS_PORT", "19092")
	defer os.Unsetenv("METRICS_PORT")

	startMetricsServer()

	resp, err := pollMetrics("http://127.0.0.1:19092/metrics", 3*time.Second)
	if err != nil {
		t.Fatalf("metrics endpoint never came up: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("metrics endpoint status: got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("expected non-empty metrics body")
	}
}

func TestStartMetricsServer_DefaultPort(t *testing.T) {
	// With METRICS_PORT unset the server defaults to :9092.
	os.Unsetenv("METRICS_PORT")

	startMetricsServer()

	resp, err := pollMetrics("http://127.0.0.1:9092/metrics", 3*time.Second)
	if err != nil {
		t.Fatalf("default-port metrics endpoint never came up: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("default-port metrics status: got %d, want 200", resp.StatusCode)
	}
}
