// White-box tests for ContextService internal state machine.
// Uses package service (not service_test) to access unexported fields.
package service

import (
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/regime"
)

// newTestService creates a minimal ContextService without live connections.
// Consumer, producer, and redis are nil — only the pure-logic methods are tested.
func newTestService() *ContextService {
	cfg := &config.Config{
		RegimeSymbols: []string{"SPY", "QQQ"},
		SectorSymbols: []string{"XLK"},
	}
	return NewContextService(cfg)
}

// makeContext builds a MarketContext with the given regime and confidence.
func makeContext(r regime.Regime, confidence float64) *regime.MarketContext {
	return &regime.MarketContext{
		Regime:           r,
		RegimeConfidence: confidence,
		Timestamp:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// ---- hasContextChanged ----------------------------------------------------

func TestHasContextChanged_NoPreviousContext_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	// lastContext is nil on a fresh service
	ctx := makeContext(regime.RegimeBull, 0.9)
	if !svc.hasContextChanged(ctx) {
		t.Error("expected true when no previous context has been published")
	}
}

func TestHasContextChanged_SameRegime_SameConfidence_ReturnsFalse(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	svc.lastPublish = time.Now() // very recent — within 5-minute window

	incoming := makeContext(regime.RegimeBull, 0.85)
	if svc.hasContextChanged(incoming) {
		t.Error("expected false when regime and confidence are unchanged and publish is recent")
	}
}

func TestHasContextChanged_RegimeChange_Bull_To_Bear_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	svc.lastPublish = time.Now()

	incoming := makeContext(regime.RegimeBear, 0.85)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true on regime change BULL → BEAR")
	}
}

func TestHasContextChanged_RegimeChange_Bull_To_Sideways_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.9)
	svc.lastPublish = time.Now()

	incoming := makeContext(regime.RegimeSideways, 0.9)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true on regime change BULL → SIDEWAYS")
	}
}

func TestHasContextChanged_ConfidenceSmallDelta_BelowThreshold_ReturnsFalse(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	svc.lastPublish = time.Now()

	// Change of 0.05 — below the 0.10 threshold
	incoming := makeContext(regime.RegimeBull, 0.90)
	if svc.hasContextChanged(incoming) {
		t.Error("expected false when confidence change is 0.05 (below 0.10 threshold)")
	}
}

func TestHasContextChanged_ConfidenceExactlyAtThreshold_ReturnsFalse(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.80)
	svc.lastPublish = time.Now()

	// Change of exactly 0.10 — the threshold is strictly greater than 0.1
	incoming := makeContext(regime.RegimeBull, 0.90)
	if svc.hasContextChanged(incoming) {
		t.Error("expected false when confidence change is exactly 0.10 (not > threshold)")
	}
}

func TestHasContextChanged_ConfidenceAboveThreshold_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.90)
	svc.lastPublish = time.Now()

	// Change of 0.15 — above the 0.10 threshold
	incoming := makeContext(regime.RegimeBull, 0.75)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true when confidence drops by 0.15 (above 0.10 threshold)")
	}
}

func TestHasContextChanged_ConfidenceIncreaseLargeEnough_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.60)
	svc.lastPublish = time.Now()

	// Confidence rises by 0.20 — above threshold in either direction
	incoming := makeContext(regime.RegimeBull, 0.80)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true when confidence increases by 0.20 (above threshold)")
	}
}

func TestHasContextChanged_StalePublish_OverFiveMinutes_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	svc.lastPublish = time.Now().Add(-6 * time.Minute) // 6 min ago — exceeds 5-min window

	// Nothing has changed, but we force-publish every 5 minutes
	incoming := makeContext(regime.RegimeBull, 0.85)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true when 5+ minutes have elapsed since last publish")
	}
}

func TestHasContextChanged_RecentPublish_FourMinutes_NoChange_ReturnsFalse(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	svc.lastPublish = time.Now().Add(-4 * time.Minute) // 4 min ago — within window

	incoming := makeContext(regime.RegimeBull, 0.85)
	if svc.hasContextChanged(incoming) {
		t.Error("expected false when < 5 minutes elapsed and nothing changed")
	}
}

func TestHasContextChanged_ExactlyFiveMinutes_ReturnsTrue(t *testing.T) {
	svc := newTestService()
	svc.lastContext = makeContext(regime.RegimeBull, 0.85)
	// Nudge past exactly 5 minutes to ensure the > comparison fires
	svc.lastPublish = time.Now().Add(-(5*time.Minute + time.Millisecond))

	incoming := makeContext(regime.RegimeBull, 0.85)
	if !svc.hasContextChanged(incoming) {
		t.Error("expected true when more than 5 minutes have elapsed")
	}
}

// ---- NewContextService / tracked symbols ----------------------------------

func TestNewContextService_TrackedSymbolsIncludeRegimeAndSector(t *testing.T) {
	cfg := &config.Config{
		RegimeSymbols: []string{"SPY", "QQQ"},
		SectorSymbols: []string{"XLK", "XLF"},
	}
	svc := NewContextService(cfg)

	for _, sym := range []string{"SPY", "QQQ", "XLK", "XLF"} {
		if !svc.trackedSymbols[sym] {
			t.Errorf("expected %s in trackedSymbols", sym)
		}
	}
	if svc.trackedSymbols["AAPL"] {
		t.Error("AAPL should not be in trackedSymbols")
	}
}

func TestNewContextService_DefaultPublishInterval_Is30Seconds(t *testing.T) {
	svc := newTestService()
	if svc.publishInterval != 30*time.Second {
		t.Errorf("publishInterval: got %v, want 30s", svc.publishInterval)
	}
}

func TestNewContextService_LastPublishIsZero(t *testing.T) {
	svc := newTestService()
	if !svc.lastPublish.IsZero() {
		t.Errorf("expected lastPublish to be zero on new service, got %v", svc.lastPublish)
	}
}

func TestNewContextService_LastContextIsNil(t *testing.T) {
	svc := newTestService()
	if svc.lastContext != nil {
		t.Errorf("expected lastContext nil on new service, got %+v", svc.lastContext)
	}
}
