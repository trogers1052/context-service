package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/trogers1052/context-service/internal/config"
)

// TestInitialize_Success drives the full Initialize happy path against an
// in-process miniredis (no Docker / external dependency). It covers the
// Redis connect-success branch and the "FRED_API_KEY not set" macro-disabled
// branch.
func TestInitialize_Success(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	port, err := strconv.Atoi(mr.Port())
	if err != nil {
		t.Fatalf("bad miniredis port %q: %v", mr.Port(), err)
	}

	cfg := &config.Config{
		KafkaBrokers:  []string{"127.0.0.1:1"}, // not contacted during Initialize
		InputTopic:    "in",
		OutputTopic:   "out",
		ConsumerGroup: "g",
		RedisHost:     mr.Host(),
		RedisPort:     port,
		ContextKey:    "cov:market:context",
		RegimeSymbols: []string{"SPY", "QQQ"},
		SectorSymbols: []string{"XLK"},
		// FREDAPIKey empty -> exercises the "macro disabled" log branch.
	}
	svc := NewContextService(cfg, nil)
	defer svc.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.Initialize(ctx); err != nil {
		t.Fatalf("Initialize against miniredis failed: %v", err)
	}

	if svc.consumer == nil || svc.producer == nil || svc.redis == nil {
		t.Error("expected consumer, producer, and redis to be wired after Initialize")
	}
	if svc.macroFetcher != nil {
		t.Error("expected macroFetcher to remain nil when FRED_API_KEY is unset")
	}
}
