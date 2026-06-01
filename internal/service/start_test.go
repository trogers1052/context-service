package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/kafka"
	"github.com/trogers1052/context-service/internal/macro"
	"github.com/trogers1052/context-service/internal/redis"
)

// fredStub serves a minimal valid FRED observations response so a Fetcher
// pointed at it can Refresh() without any live network dependency.
func fredStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"observations":[{"date":"2026-03-14","value":"18.50"}]}`))
	}))
}

func TestStart_NoMacro_ContextCancelled_ReturnsPromptly(t *testing.T) {
	svc := newTestService()
	// Real consumer pointed at a dead broker; Start calls consumer.Start which
	// returns ctx.Err() promptly when the context is already cancelled.
	svc.consumer = kafka.NewConsumer([]string{"127.0.0.1:1"}, "in", "g", svc.handleMessage)
	svc.producer = kafka.NewProducer([]string{"127.0.0.1:1"}, "out")
	svc.redis = redis.NewClient("127.0.0.1", 1, "", 0, "k")
	defer svc.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected ctx error from Start with cancelled context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return promptly on cancelled context")
	}
}

func TestStart_WithMacroFetcher_SpawnsRefresher(t *testing.T) {
	stub := fredStub(t)
	defer stub.Close()

	svc := newTestService()
	svc.consumer = kafka.NewConsumer([]string{"127.0.0.1:1"}, "in", "g", svc.handleMessage)
	svc.producer = kafka.NewProducer([]string{"127.0.0.1:1"}, "out")
	svc.redis = redis.NewClient("127.0.0.1", 1, "", 0, "k")
	// Attaching a macroFetcher makes Start spawn runMacroRefresher.
	svc.macroFetcher = macro.NewFetcherWithBaseURL("test-key", stub.URL)
	defer svc.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return promptly on cancelled context (with macro)")
	}
}
