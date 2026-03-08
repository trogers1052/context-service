// Tests for handleMessage and maybePublishContext/abs.
// Uses package service (not service_test) to access unexported fields.
package service

import (
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/regime"
)

// ---- handleMessage ----------------------------------------------------------

func TestHandleMessage_ValidIndicatorEvent_UpdatesDetector(t *testing.T) {
	svc := newTestService()
	svc.lastAttempt = time.Now() // block rate limiter so publish path is skipped

	msg := `{
		"event_type": "indicator_update",
		"data": {
			"symbol": "SPY",
			"timestamp": "2026-02-20T10:00:00Z",
			"close": 500.0,
			"sma_20": 495.0,
			"sma_50": 480.0,
			"sma_200": 450.0,
			"rsi_14": 60.5,
			"macd": 2.5,
			"macd_signal": 1.8,
			"macd_histogram": 0.7,
			"atr_14": 5.0,
			"volume": 1000000,
			"volume_sma_20": 900000
		}
	}`

	err := svc.handleMessage([]byte("SPY"), []byte(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the detector got the indicators
	ind := svc.detector.GetIndicators("SPY")
	if ind == nil {
		t.Fatal("expected indicators to be stored in detector")
	}
	if ind.Close != 500.0 {
		t.Errorf("Close: got %.2f, want 500.00", ind.Close)
	}
	if ind.RSI14 != 60.5 {
		t.Errorf("RSI14: got %.2f, want 60.50", ind.RSI14)
	}
	if ind.SMA200 != 450.0 {
		t.Errorf("SMA200: got %.2f, want 450.00", ind.SMA200)
	}
	if ind.MACD != 2.5 {
		t.Errorf("MACD: got %.2f, want 2.50", ind.MACD)
	}
	if ind.Volume != 1000000 {
		t.Errorf("Volume: got %d, want 1000000", ind.Volume)
	}
}

func TestHandleMessage_UntrackedSymbol_Ignored(t *testing.T) {
	svc := newTestService()

	// AAPL is not in trackedSymbols (only SPY, QQQ, XLK)
	msg := `{
		"event_type": "indicator_update",
		"data": {
			"symbol": "AAPL",
			"close": 180.0,
			"sma_200": 170.0,
			"rsi_14": 55.0,
			"macd": 1.0,
			"macd_signal": 0.8
		}
	}`

	err := svc.handleMessage([]byte("AAPL"), []byte(msg))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ind := svc.detector.GetIndicators("AAPL")
	if ind != nil {
		t.Error("expected AAPL indicators to NOT be stored (untracked symbol)")
	}
}

func TestHandleMessage_InvalidJSON_ReturnsNilError(t *testing.T) {
	svc := newTestService()

	// Invalid JSON should not return an error (logged but not blocking)
	err := svc.handleMessage([]byte("key"), []byte("not-json"))
	if err != nil {
		t.Errorf("expected nil error for invalid JSON, got %v", err)
	}
}

func TestHandleMessage_EmptyBody_ReturnsNilError(t *testing.T) {
	svc := newTestService()

	err := svc.handleMessage([]byte("key"), []byte(""))
	if err != nil {
		t.Errorf("expected nil error for empty body, got %v", err)
	}
}

func TestHandleMessage_AllFieldsMapped(t *testing.T) {
	svc := newTestService()
	svc.lastAttempt = time.Now() // block rate limiter

	msg := `{
		"event_type": "indicator_update",
		"data": {
			"symbol": "QQQ",
			"timestamp": "2026-02-20T10:00:00Z",
			"close": 420.0,
			"volume": 500000,
			"sma_20": 415.0,
			"sma_50": 410.0,
			"sma_200": 380.0,
			"rsi_14": 55.0,
			"macd": 3.0,
			"macd_signal": 2.0,
			"macd_histogram": 1.0,
			"atr_14": 4.5,
			"volume_sma_20": 480000.0
		}
	}`

	svc.handleMessage([]byte("QQQ"), []byte(msg))

	ind := svc.detector.GetIndicators("QQQ")
	if ind == nil {
		t.Fatal("expected QQQ indicators")
	}
	if ind.SMA20 != 415.0 {
		t.Errorf("SMA20: got %.2f, want 415.00", ind.SMA20)
	}
	if ind.SMA50 != 410.0 {
		t.Errorf("SMA50: got %.2f, want 410.00", ind.SMA50)
	}
	if ind.MACDHistogram != 1.0 {
		t.Errorf("MACDHistogram: got %.2f, want 1.00", ind.MACDHistogram)
	}
	if ind.ATR14 != 4.5 {
		t.Errorf("ATR14: got %.2f, want 4.50", ind.ATR14)
	}
	if ind.VolumeSMA20 != 480000.0 {
		t.Errorf("VolumeSMA20: got %.2f, want 480000.00", ind.VolumeSMA20)
	}
}

func TestHandleMessage_SectorSymbol_Tracked(t *testing.T) {
	svc := newTestService()
	svc.lastAttempt = time.Now() // block rate limiter

	msg := `{
		"event_type": "indicator_update",
		"data": {
			"symbol": "XLK",
			"close": 200.0,
			"sma_200": 180.0,
			"rsi_14": 60.0,
			"macd": 1.5,
			"macd_signal": 1.0
		}
	}`

	svc.handleMessage([]byte("XLK"), []byte(msg))

	ind := svc.detector.GetIndicators("XLK")
	if ind == nil {
		t.Error("expected XLK (sector symbol) to be tracked and stored")
	}
}

// ---- maybePublishContext rate limiting ---------------------------------------

func TestMaybePublishContext_RateLimited_ReturnsQuickly(t *testing.T) {
	svc := newTestService()
	// Simulate a recent attempt
	svc.lastAttempt = time.Now()

	// Should return immediately due to rate limiting (no panic, no nil deref)
	svc.maybePublishContext()
}

func TestMaybePublishContext_InsufficientData_NoPublish(t *testing.T) {
	svc := newTestService()
	// lastAttempt is zero, so rate limit won't block
	// But detector has no data → HasSufficientData returns false
	svc.maybePublishContext()

	// Verify no context was published
	if svc.lastContext != nil {
		t.Error("expected lastContext to remain nil when insufficient data")
	}
}

// ---- abs helper -------------------------------------------------------------

func TestAbs_PositiveNumber(t *testing.T) {
	if abs(5.0) != 5.0 {
		t.Errorf("abs(5.0) = %.2f, want 5.00", abs(5.0))
	}
}

func TestAbs_NegativeNumber(t *testing.T) {
	if abs(-3.14) != 3.14 {
		t.Errorf("abs(-3.14) = %.2f, want 3.14", abs(-3.14))
	}
}

func TestAbs_Zero(t *testing.T) {
	if abs(0.0) != 0.0 {
		t.Errorf("abs(0.0) = %.2f, want 0.00", abs(0.0))
	}
}

// ---- AnalyzeSymbol missing data edge cases (via detector) -------------------

func TestAnalyzeSymbol_ZeroSMA200_ZeroRSI_ReturnsUnknown(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(&regime.Indicators{
		Symbol:     "SPY",
		Close:      500,
		SMA200:     0,
		RSI14:      0,
		MACD:       2,
		MACDSignal: 1,
	})

	sr := d.AnalyzeSymbol("SPY")
	if sr == nil {
		t.Fatal("expected non-nil result")
	}
	if sr.Regime != regime.RegimeUnknown {
		t.Errorf("regime: got %s, want UNKNOWN when both SMA200 and RSI are zero", sr.Regime)
	}
	if sr.Confidence != 0.0 {
		t.Errorf("confidence: got %.2f, want 0.00", sr.Confidence)
	}
}

func TestAnalyzeSymbol_ZeroSMA200_ValidRSI_ReducedConfidence(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(&regime.Indicators{
		Symbol:     "SPY",
		Close:      500,
		SMA200:     0, // missing
		RSI14:      60,
		MACD:       2,
		MACDSignal: 1,
	})

	sr := d.AnalyzeSymbol("SPY")
	if sr == nil {
		t.Fatal("expected non-nil result")
	}
	// 2 valid indicators (RSI + MACD), both bullish → 2/2 = 1.0 → BULL 0.9
	// But validCount=2 < 3, so confidence = 0.9 * 2/3 = 0.6
	if sr.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", sr.Regime)
	}
	if sr.Confidence != 0.6 {
		t.Errorf("confidence: got %.4f, want 0.6000 (reduced for missing SMA200)", sr.Confidence)
	}
}

func TestAnalyzeSymbol_ValidSMA200_ZeroRSI(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(&regime.Indicators{
		Symbol:     "SPY",
		Close:      500,
		SMA200:     400,
		RSI14:      0, // missing
		MACD:       2,
		MACDSignal: 1,
	})

	sr := d.AnalyzeSymbol("SPY")
	if sr == nil {
		t.Fatal("expected non-nil result")
	}
	// 2 valid: SMA200 (bullish: 500>400) + MACD (bullish: 2>1) → 2/2 = 1.0 → BULL 0.9
	// validCount=2 < 3, so confidence = 0.9 * 2/3 = 0.6
	if sr.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", sr.Regime)
	}
	if sr.Confidence != 0.6 {
		t.Errorf("confidence: got %.4f, want 0.6000", sr.Confidence)
	}
}

// ---- Stop (nil-safe) -------------------------------------------------------

func TestStop_NilConnections_DoesNotPanic(t *testing.T) {
	cfg := &config.Config{
		RegimeSymbols: []string{"SPY"},
		SectorSymbols: []string{},
	}
	svc := NewContextService(cfg)
	// consumer, producer, redis are all nil
	// Stop should handle nil gracefully
	svc.Stop()
}
