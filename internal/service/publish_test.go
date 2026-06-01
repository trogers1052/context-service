package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/trogers1052/context-service/internal/kafka"
	"github.com/trogers1052/context-service/internal/redis"
	"github.com/trogers1052/context-service/internal/regime"
)

// wireDeadIO attaches a live in-process miniredis (so the Redis write succeeds)
// and a Kafka producer pointed at a dead broker (so the Kafka write fails but
// is tolerated). Together they exercise both the success and failure branches
// of publishContext without any external dependency. The returned cleanup
// closes the miniredis.
func wireDeadIO(t *testing.T, svc *ContextService) func() {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	port, _ := strconv.Atoi(mr.Port())
	svc.redis = redis.NewClient(mr.Host(), port, "", 0, "market:context")
	if connErr := svc.redis.Connect(context.Background()); connErr != nil {
		mr.Close()
		t.Fatalf("Connect to miniredis failed: %v", connErr)
	}
	svc.producer = kafka.NewProducer([]string{"127.0.0.1:1"}, "out")
	return mr.Close
}

// feedSufficientData pushes SPY+QQQ indicators so HasSufficientData() is true
// and GetMarketContext() produces a real regime.
func feedSufficientData(svc *ContextService) {
	for _, sym := range []string{"SPY", "QQQ"} {
		svc.detector.UpdateIndicators(&regime.Indicators{
			Symbol:     sym,
			Close:      450,
			SMA20:      440,
			SMA200:     420,
			RSI14:      60,
			MACD:       1.0,
			MACDSignal: 0.5,
			Timestamp:  time.Now(),
		})
	}
}

func TestPublishContext_DeadIO_StillUpdatesLastContext(t *testing.T) {
	svc := newTestService()
	cleanup := wireDeadIO(t, svc)
	defer cleanup()
	defer svc.Stop()

	svc.ctx = context.Background()
	feedSufficientData(svc)
	marketCtx := svc.detector.GetMarketContext()

	// Pre-condition: no last context yet.
	if svc.lastContext != nil {
		t.Fatal("expected nil lastContext before publish")
	}

	svc.publishContext(marketCtx) // dead I/O — should log errors but not panic

	if svc.lastContext == nil {
		t.Error("expected lastContext set even when Redis/Kafka writes fail")
	}
	if svc.lastPublish.IsZero() {
		t.Error("expected lastPublish updated after publishContext")
	}
}

func TestPublishContext_NilCtx_UsesBackground(t *testing.T) {
	svc := newTestService()
	cleanup := wireDeadIO(t, svc)
	defer cleanup()
	defer svc.Stop()

	svc.ctx = nil // force the ctx==nil -> context.Background() branch
	feedSufficientData(svc)
	marketCtx := svc.detector.GetMarketContext()

	svc.publishContext(marketCtx) // must not panic with nil svc.ctx
	if svc.lastContext == nil {
		t.Error("expected lastContext set after publish with nil ctx")
	}
}

func TestPublishContext_HeartbeatReason_SameRegime(t *testing.T) {
	svc := newTestService()
	cleanup := wireDeadIO(t, svc)
	defer cleanup()
	defer svc.Stop()
	svc.ctx = context.Background()

	feedSufficientData(svc)
	first := svc.detector.GetMarketContext()
	svc.publishContext(first) // sets lastContext

	// Publish again with same regime -> reason == "heartbeat" branch.
	second := svc.detector.GetMarketContext()
	svc.publishContext(second)

	if svc.lastContext == nil {
		t.Error("expected lastContext still set after heartbeat publish")
	}
}

func TestMaybePublishContext_FullPath_Publishes(t *testing.T) {
	svc := newTestService()
	cleanup := wireDeadIO(t, svc)
	defer cleanup()
	defer svc.Stop()
	svc.ctx = context.Background()

	// No recent attempt so rate-limit gate passes.
	svc.lastAttempt = time.Time{}
	feedSufficientData(svc)

	svc.maybePublishContext()

	if svc.lastContext == nil {
		t.Error("expected maybePublishContext to publish (lastContext set)")
	}
}

func TestMaybePublishContext_NoChange_DoesNotRepublish(t *testing.T) {
	svc := newTestService()
	cleanup := wireDeadIO(t, svc)
	defer cleanup()
	defer svc.Stop()
	svc.ctx = context.Background()
	feedSufficientData(svc)

	// First publish.
	svc.lastAttempt = time.Time{}
	svc.maybePublishContext()
	firstPublish := svc.lastPublish

	// Reset the rate-limit gate but keep lastPublish recent and regime unchanged
	// -> hasContextChanged() returns false, so no republish.
	svc.lastAttempt = time.Time{}
	svc.maybePublishContext()

	if !svc.lastPublish.Equal(firstPublish) {
		t.Error("expected no republish when context is unchanged and recent")
	}
}
