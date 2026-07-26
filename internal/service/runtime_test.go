package service

import (
	"context"
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/regime"
)

// ---- extractIndicators ------------------------------------------------------

func TestExtractIndicators_MapsUppercaseKeys(t *testing.T) {
	d := &IndicatorData{
		Symbol: "SPY",
		Time:   "2026-03-14T15:04:05Z",
		Indicators: map[string]float64{
			"close":          450.5,
			"volume":         1234567,
			"SMA_20":         448.0,
			"SMA_50":         440.0,
			"SMA_200":        420.0,
			"RSI_14":         62.5,
			"MACD":           1.2,
			"MACD_SIGNAL":    0.9,
			"MACD_HISTOGRAM": 0.3,
			"ATR_14":         5.5,
			"volume_sma_20":  1000000,
		},
	}
	p := extractIndicators(d)

	if p.Close != 450.5 {
		t.Errorf("Close: got %v, want 450.5", p.Close)
	}
	if p.Volume != 1234567 {
		t.Errorf("Volume: got %v, want 1234567", p.Volume)
	}
	if p.SMA200 != 420.0 {
		t.Errorf("SMA200: got %v, want 420.0", p.SMA200)
	}
	if p.RSI14 != 62.5 {
		t.Errorf("RSI14: got %v, want 62.5", p.RSI14)
	}
	if p.MACDSignal != 0.9 {
		t.Errorf("MACDSignal: got %v, want 0.9", p.MACDSignal)
	}
	if p.Timestamp.IsZero() {
		t.Error("expected Timestamp to be parsed from Time")
	}
}

func TestExtractIndicators_MissingKeys_ZeroValues(t *testing.T) {
	d := &IndicatorData{Symbol: "QQQ", Indicators: map[string]float64{"close": 380}}
	p := extractIndicators(d)
	if p.Close != 380 {
		t.Errorf("Close: got %v, want 380", p.Close)
	}
	if p.SMA200 != 0 || p.RSI14 != 0 {
		t.Errorf("expected missing indicators to be zero, got SMA200=%v RSI14=%v", p.SMA200, p.RSI14)
	}
}

func TestExtractIndicators_EmptyTime_ZeroTimestamp(t *testing.T) {
	d := &IndicatorData{Symbol: "SPY", Time: "", Indicators: map[string]float64{}}
	p := extractIndicators(d)
	if !p.Timestamp.IsZero() {
		t.Errorf("expected zero timestamp for empty Time, got %v", p.Timestamp)
	}
}

func TestExtractIndicators_InvalidTime_ZeroTimestamp(t *testing.T) {
	d := &IndicatorData{Symbol: "SPY", Time: "not-a-timestamp", Indicators: map[string]float64{}}
	p := extractIndicators(d)
	if !p.Timestamp.IsZero() {
		t.Errorf("expected zero timestamp for invalid Time, got %v", p.Timestamp)
	}
}

// ---- abs --------------------------------------------------------------------

func TestAbs(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{1.5, 1.5},
		{-1.5, 1.5},
		{-0.0001, 0.0001},
	}
	for _, c := range cases {
		if got := abs(c.in); got != c.want {
			t.Errorf("abs(%v): got %v, want %v", c.in, got, c.want)
		}
	}
}

// ---- maybePublishContext ----------------------------------------------------

func TestMaybePublishContext_RateLimited_ReturnsEarly(t *testing.T) {
	svc := newTestService()
	// Recent attempt within the 30s publishInterval — should bail before
	// touching the detector or any nil connection.
	svc.lastAttempt = time.Now()

	// Must not panic even though consumer/producer/redis are nil.
	svc.maybePublishContext()
}

func TestMaybePublishContext_InsufficientData_ReturnsEarly(t *testing.T) {
	svc := newTestService()
	// Allow the rate-limit gate to pass (no recent attempt), but the detector
	// has no data, so HasSufficientData() is false and we return before any I/O.
	svc.lastAttempt = time.Time{}

	svc.maybePublishContext() // no panic, no publish
}

// ---- handleMessage non-tracked symbol ---------------------------------------

func TestHandleMessage_UntrackedSymbol_NoPublish(t *testing.T) {
	svc := newTestService()
	// AAPL is not in regime or sector symbols, so it's dropped before any I/O.
	msg := []byte(`{"event_type":"indicator","data":{"symbol":"AAPL","indicators":{"close":150}}}`)
	if err := svc.handleMessage(nil, msg); err != nil {
		t.Errorf("expected nil error for untracked symbol, got %v", err)
	}
}

func TestHandleMessage_MalformedJSON_ReturnsNil(t *testing.T) {
	svc := newTestService()
	if err := svc.handleMessage(nil, []byte("{not json")); err != nil {
		t.Errorf("expected nil (swallowed) error for malformed JSON, got %v", err)
	}
}

func TestHandleMessage_TrackedSymbol_RateLimited_NoPanic(t *testing.T) {
	svc := newTestService()
	// Prevent maybePublishContext from reaching I/O by tripping the rate-limit gate.
	svc.lastAttempt = time.Now()
	msg := []byte(`{"event_type":"indicator","data":{"symbol":"SPY","time":"2026-03-14T15:04:05Z","indicators":{"close":450,"SMA_200":420,"RSI_14":60,"MACD":1.0,"MACD_SIGNAL":0.5}}}`)
	if err := svc.handleMessage(nil, msg); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	// Detector should now hold SPY data.
	if got := svc.detector.GetIndicators("SPY"); got == nil {
		t.Error("expected SPY indicators stored in detector after handleMessage")
	}
}

// ---- Stop -------------------------------------------------------------------

func TestStop_NilDependencies_NoPanic(t *testing.T) {
	svc := newTestService()
	// consumer/producer/redis are all nil; Stop must guard each.
	svc.Stop()
}

// ---- runMacroRefresher ------------------------------------------------------

func TestRunMacroRefresher_ContextCancelled_Returns(t *testing.T) {
	svc := newTestService()
	svc.wg.Add(1) // runMacroRefresher calls wg.Done() on exit

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		svc.runMacroRefresher(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runMacroRefresher did not exit on cancelled context")
	}
}

// ---- Initialize (redis connect failure) -------------------------------------

func TestInitialize_RedisConnectFails_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		KafkaBrokers:  []string{"127.0.0.1:1"},
		InputTopic:    "in",
		OutputTopic:   "out",
		ConsumerGroup: "g",
		RedisHost:     "127.0.0.1",
		RedisPort:     1, // dead port — Connect's Ping fails fast
		ContextKey:    "market:context",
		RegimeSymbols: []string{"SPY"},
		// FREDAPIKey empty so macro init is skipped.
	}
	svc := NewContextService(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := svc.Initialize(ctx); err == nil {
		t.Error("expected Initialize to fail when Redis is unreachable")
	}
	// Clean up the consumer/producer Initialize created.
	svc.Stop()
}

// ---- sanity: ensure MarketContext type is usable in this package ------------

func TestMakeContext_Helper(t *testing.T) {
	c := makeContext(regime.RegimeBear, 0.42)
	if c.Regime != regime.RegimeBear || c.RegimeConfidence != 0.42 {
		t.Errorf("makeContext: got %v/%v", c.Regime, c.RegimeConfidence)
	}
}
