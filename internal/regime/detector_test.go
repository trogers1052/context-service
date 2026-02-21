package regime_test

import (
	"math"
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/regime"
)

// makeIndicators builds an Indicators value with only the fields used by regime
// detection set. SMA50 is set to a non-zero sentinel so callers don't have to
// worry about it; all unused fields default to zero.
func makeIndicators(symbol string, close, sma20, sma200, rsi14, macd, macdSignal float64) *regime.Indicators {
	return &regime.Indicators{
		Symbol:     symbol,
		Close:      close,
		SMA20:      sma20,
		SMA50:      sma200 * 0.99, // not used by regime logic, just non-zero
		SMA200:     sma200,
		RSI14:      rsi14,
		MACD:       macd,
		MACDSignal: macdSignal,
		Timestamp:  time.Now(),
	}
}

// ---- AnalyzeSymbol --------------------------------------------------------

func TestAnalyzeSymbol_AllBullish_ReturnsBull090(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// close > SMA200 ✓  RSI > 50 ✓  MACD > Signal ✓  → 3/3 → BULL 0.90
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1))

	sr := d.AnalyzeSymbol("SPY")
	if sr == nil {
		t.Fatal("expected non-nil SymbolRegime")
	}
	if sr.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", sr.Regime)
	}
	if sr.Confidence != 0.9 {
		t.Errorf("confidence: got %.2f, want 0.90", sr.Confidence)
	}
	if !sr.AboveSMA200 || !sr.RSIBullish || !sr.MACDBullish {
		t.Errorf("condition flags: above200=%v rsi=%v macd=%v; want all true",
			sr.AboveSMA200, sr.RSIBullish, sr.MACDBullish)
	}
}

func TestAnalyzeSymbol_AllBearish_ReturnsBear090(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// close < SMA200 ✗  RSI < 50 ✗  MACD < Signal ✗  → 0/3 → BEAR 0.90
	d.UpdateIndicators(makeIndicators("SPY", 380, 370, 400, 45, 1, 2))

	sr := d.AnalyzeSymbol("SPY")
	if sr.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", sr.Regime)
	}
	if sr.Confidence != 0.9 {
		t.Errorf("confidence: got %.2f, want 0.90", sr.Confidence)
	}
	if sr.AboveSMA200 || sr.RSIBullish || sr.MACDBullish {
		t.Errorf("condition flags: above200=%v rsi=%v macd=%v; want all false",
			sr.AboveSMA200, sr.RSIBullish, sr.MACDBullish)
	}
}

func TestAnalyzeSymbol_TwoBullish_WithSMA200_ReturnsBull070(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// close > SMA200 ✓  RSI > 50 ✓  MACD < Signal ✗  → 2/3 with SMA200 → BULL 0.70
	d.UpdateIndicators(makeIndicators("SPY", 450, 440, 400, 55, 1, 2))

	sr := d.AnalyzeSymbol("SPY")
	if sr.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", sr.Regime)
	}
	if sr.Confidence != 0.7 {
		t.Errorf("confidence: got %.2f, want 0.70", sr.Confidence)
	}
}

func TestAnalyzeSymbol_TwoBullish_WithoutSMA200_ReturnsSideways060(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// close < SMA200 ✗  RSI > 50 ✓  MACD > Signal ✓  → 2/3 without SMA200 → SIDEWAYS 0.60
	d.UpdateIndicators(makeIndicators("SPY", 390, 385, 400, 55, 2, 1))

	sr := d.AnalyzeSymbol("SPY")
	if sr.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS", sr.Regime)
	}
	if sr.Confidence != 0.6 {
		t.Errorf("confidence: got %.2f, want 0.60", sr.Confidence)
	}
}

func TestAnalyzeSymbol_OneBullish_ReturnsSideways050(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// close < SMA200 ✗  RSI < 50 ✗  MACD > Signal ✓  → 1/3 → SIDEWAYS 0.50
	d.UpdateIndicators(makeIndicators("SPY", 380, 375, 400, 45, 2, 1))

	sr := d.AnalyzeSymbol("SPY")
	if sr.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS", sr.Regime)
	}
	if sr.Confidence != 0.5 {
		t.Errorf("confidence: got %.2f, want 0.50", sr.Confidence)
	}
}

func TestAnalyzeSymbol_MissingSymbol_ReturnsNil(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// No indicators loaded for SPY — should return nil, not panic
	sr := d.AnalyzeSymbol("SPY")
	if sr != nil {
		t.Errorf("expected nil for symbol with no data, got %+v", sr)
	}
}

func TestAnalyzeSymbol_UnknownSymbol_ReturnsNil(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1))

	sr := d.AnalyzeSymbol("QQQ") // not loaded
	if sr != nil {
		t.Errorf("expected nil for unknown symbol, got %+v", sr)
	}
}

func TestAnalyzeSymbol_TrendStrength_TenPctAboveSMA200(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	// 10% above SMA200: (440-400)/400*100 = 10.0
	d.UpdateIndicators(makeIndicators("SPY", 440, 430, 400, 60, 2, 1))

	sr := d.AnalyzeSymbol("SPY")
	want := ((440.0 - 400.0) / 400.0) * 100 // 10.0
	if math.Abs(sr.TrendStrength-want) > 0.001 {
		t.Errorf("TrendStrength: got %.4f, want %.4f", sr.TrendStrength, want)
	}
}

func TestAnalyzeSymbol_TrendStrength_ZeroSMA200_IsZero(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 440, 430, 0, 60, 2, 1)) // SMA200 = 0

	sr := d.AnalyzeSymbol("SPY")
	if sr.TrendStrength != 0 {
		t.Errorf("TrendStrength: got %.4f, want 0 when SMA200=0", sr.TrendStrength)
	}
}

// ---- HasSufficientData ----------------------------------------------------

func TestHasSufficientData_NoIndicators_ReturnsFalse(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	if d.HasSufficientData() {
		t.Error("expected false when no indicators loaded")
	}
}

func TestHasSufficientData_SPYWithZeroSMA200_ReturnsFalse(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 440, 430, 0, 60, 2, 1)) // SMA200 = 0

	if d.HasSufficientData() {
		t.Error("expected false when SMA200 is zero (insufficient history)")
	}
}

func TestHasSufficientData_SPYWithValidSMA200_ReturnsTrue(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 440, 430, 400, 60, 2, 1))

	if !d.HasSufficientData() {
		t.Error("expected true when SPY has valid SMA200")
	}
}

func TestHasSufficientData_OnlyQQQ_ReturnsFalse(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	// SPY is required; QQQ alone is not sufficient
	d.UpdateIndicators(makeIndicators("QQQ", 420, 410, 350, 60, 2, 1))

	if d.HasSufficientData() {
		t.Error("expected false when SPY is missing (only QQQ loaded)")
	}
}

// ---- GetMarketContext — overall regime combination ------------------------

func TestGetMarketContext_BothBull_ReturnsBull090(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1)) // BULL 0.9
	d.UpdateIndicators(makeIndicators("QQQ", 420, 410, 350, 60, 2, 1)) // BULL 0.9

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", ctx.Regime)
	}
	// Both agree at 0.9 → avg = 0.9
	if math.Abs(ctx.RegimeConfidence-0.9) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.9000", ctx.RegimeConfidence)
	}
	if ctx.SPYRegime == nil || ctx.QQQRegime == nil {
		t.Error("expected both SPYRegime and QQQRegime populated")
	}
}

func TestGetMarketContext_BothBear_ReturnsBear090(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 350, 360, 400, 45, 1, 2)) // BEAR 0.9
	d.UpdateIndicators(makeIndicators("QQQ", 290, 300, 340, 45, 1, 2)) // BEAR 0.9

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	if math.Abs(ctx.RegimeConfidence-0.9) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.9000", ctx.RegimeConfidence)
	}
}

func TestGetMarketContext_SPYBull_QQQBear_ReturnsSideways050(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1)) // BULL
	d.UpdateIndicators(makeIndicators("QQQ", 290, 300, 340, 45, 1, 2)) // BEAR

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS on BULL/BEAR disagreement", ctx.Regime)
	}
	if ctx.RegimeConfidence != 0.5 {
		t.Errorf("confidence: got %.2f, want 0.50", ctx.RegimeConfidence)
	}
}

func TestGetMarketContext_SPYOnly_PenalisedConfidence(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1)) // BULL 0.9

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", ctx.Regime)
	}
	// SPY-only penalty: 0.9 * 0.8 = 0.72
	want := 0.9 * 0.8
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (SPY-only 0.8× penalty)", ctx.RegimeConfidence, want)
	}
	if ctx.QQQRegime != nil {
		t.Error("expected nil QQQRegime when QQQ has no data")
	}
}

func TestGetMarketContext_QQQOnly_PenalisedConfidence(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	d.UpdateIndicators(makeIndicators("QQQ", 420, 410, 350, 60, 2, 1)) // BULL 0.9

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", ctx.Regime)
	}
	// QQQ-only penalty: 0.9 * 0.7 = 0.63
	want := 0.9 * 0.7
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (QQQ-only 0.7× penalty)", ctx.RegimeConfidence, want)
	}
	if ctx.SPYRegime != nil {
		t.Error("expected nil SPYRegime when SPY has no data")
	}
}

func TestGetMarketContext_NoData_ReturnsUnknown(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)

	ctx := d.GetMarketContext()

	if ctx.Regime != regime.RegimeUnknown {
		t.Errorf("regime: got %s, want UNKNOWN when no indicators loaded", ctx.Regime)
	}
}

func TestGetMarketContext_HasTimestamp(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	before := time.Now()
	ctx := d.GetMarketContext()
	after := time.Now()

	if ctx.Timestamp.Before(before) || ctx.Timestamp.After(after) {
		t.Errorf("Timestamp %v not in expected range [%v, %v]", ctx.Timestamp, before, after)
	}
}

// ---- combineRegimes edge cases (tested indirectly via GetMarketContext) ---

func TestGetMarketContext_SPYSideways_QQQBull_LeansQQQ(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	// SPY: 1/3 → SIDEWAYS 0.5
	d.UpdateIndicators(makeIndicators("SPY", 380, 375, 400, 45, 2, 1))
	// QQQ: 3/3 → BULL 0.9
	d.UpdateIndicators(makeIndicators("QQQ", 420, 410, 350, 60, 2, 1))

	ctx := d.GetMarketContext()

	// SPY SIDEWAYS + QQQ BULL → leans toward QQQ (directional) with 0.7× penalty
	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL when SPY sideways and QQQ bull", ctx.Regime)
	}
	want := 0.9 * 0.7
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f", ctx.RegimeConfidence, want)
	}
}

func TestGetMarketContext_SPYBull_QQQSideways_LeansSPY(t *testing.T) {
	d := regime.NewDetector([]string{"SPY", "QQQ"}, nil)
	// SPY: close > SMA200 ✓, RSI > 50 ✓, MACD > Signal ✓  → 3/3 → BULL 0.9
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1))
	// QQQ: close < SMA200 ✗, RSI < 50 ✗, MACD > Signal ✓  → 1/3 → SIDEWAYS 0.5
	// close=330 < sma200=350 ensures below-SMA200 (aboveSMA200 = false)
	d.UpdateIndicators(makeIndicators("QQQ", 330, 320, 350, 45, 2, 1))

	ctx := d.GetMarketContext()

	// SPY BULL + QQQ SIDEWAYS → combineRegimes leans toward SPY with 0.8× penalty
	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL when SPY bull and QQQ sideways", ctx.Regime)
	}
	want := 0.9 * 0.8
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f", ctx.RegimeConfidence, want)
	}
}

// ---- Sector strength ------------------------------------------------------

func TestGetMarketContext_SectorLeader(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, []string{"XLK"})
	// SPY: close=500, sma20=495 → trend = (500-495)/495 ≈ 0.01010
	// XLK: close=200, sma20=194 → trend = (200-194)/194 ≈ 0.03093
	// relative = (0.03093 - 0.01010) * 100 ≈ +2.08 → leader (> +1.0)
	d.UpdateIndicators(makeIndicators("SPY", 500, 495, 400, 60, 2, 1))
	d.UpdateIndicators(makeIndicators("XLK", 200, 194, 160, 60, 2, 1))

	ctx := d.GetMarketContext()

	if len(ctx.SectorLeaders) != 1 || ctx.SectorLeaders[0] != "XLK" {
		t.Errorf("leaders: got %v, want [XLK]", ctx.SectorLeaders)
	}
	if len(ctx.SectorLaggards) != 0 {
		t.Errorf("laggards: got %v, want []", ctx.SectorLaggards)
	}
}

func TestGetMarketContext_SectorLaggard(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, []string{"XLE"})
	// SPY: close=500, sma20=495 → trend ≈ +0.01010
	// XLE: close=80, sma20=83 → trend = (80-83)/83 ≈ -0.03614
	// relative = (-0.03614 - 0.01010) * 100 ≈ -4.62 → laggard (< -1.0)
	d.UpdateIndicators(makeIndicators("SPY", 500, 495, 400, 60, 2, 1))
	d.UpdateIndicators(makeIndicators("XLE", 80, 83, 65, 40, 1, 2))

	ctx := d.GetMarketContext()

	if len(ctx.SectorLaggards) != 1 || ctx.SectorLaggards[0] != "XLE" {
		t.Errorf("laggards: got %v, want [XLE]", ctx.SectorLaggards)
	}
	if len(ctx.SectorLeaders) != 0 {
		t.Errorf("leaders: got %v, want []", ctx.SectorLeaders)
	}
}

func TestGetMarketContext_SectorNeutral_NotInEitherList(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, []string{"XLF"})
	// SPY: close=500, sma20=495 → trend ≈ +0.01010
	// XLF: close=100, sma20=99 → trend = (100-99)/99 ≈ +0.01010
	// relative ≈ 0.0 → neutral (neither > +1.0 nor < -1.0)
	d.UpdateIndicators(makeIndicators("SPY", 500, 495, 400, 60, 2, 1))
	d.UpdateIndicators(makeIndicators("XLF", 100, 99, 80, 52, 2, 1))

	ctx := d.GetMarketContext()

	if len(ctx.SectorLeaders) != 0 || len(ctx.SectorLaggards) != 0 {
		t.Errorf("expected neutral sector: leaders=%v laggards=%v", ctx.SectorLeaders, ctx.SectorLaggards)
	}
	if _, ok := ctx.SectorStrength["XLF"]; !ok {
		t.Error("expected XLF to appear in SectorStrength map")
	}
}

func TestGetMarketContext_SectorNoSPY_SectorStrengthSkipped(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, []string{"XLK"})
	// XLK loaded but SPY has no data → calculateSectorStrength bails early
	d.UpdateIndicators(makeIndicators("XLK", 200, 194, 160, 60, 2, 1))

	ctx := d.GetMarketContext()

	if len(ctx.SectorStrength) != 0 {
		t.Errorf("expected empty SectorStrength when SPY missing, got %v", ctx.SectorStrength)
	}
}

func TestGetMarketContext_SectorMissingData_Skipped(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, []string{"XLK", "XLE"})
	// Only SPY loaded — XLK and XLE have no indicators
	d.UpdateIndicators(makeIndicators("SPY", 500, 495, 400, 60, 2, 1))

	ctx := d.GetMarketContext()

	if len(ctx.SectorStrength) != 0 {
		t.Errorf("expected empty SectorStrength when sectors have no data, got %v", ctx.SectorStrength)
	}
}

// ---- UpdateIndicators / GetIndicators -------------------------------------

func TestUpdateAndGetIndicators_RoundTrip(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	ind := makeIndicators("SPY", 500, 490, 400, 60, 2, 1)
	d.UpdateIndicators(ind)

	got := d.GetIndicators("SPY")
	if got == nil {
		t.Fatal("GetIndicators returned nil after UpdateIndicators")
	}
	if got.Symbol != "SPY" || got.Close != 500 || got.SMA200 != 400 {
		t.Errorf("unexpected indicator values: %+v", got)
	}
}

func TestGetIndicators_MissingSymbol_ReturnsNil(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	if got := d.GetIndicators("NOPE"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpdateIndicators_OverwritesPreviousValue(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(makeIndicators("SPY", 400, 390, 350, 45, 1, 2))
	d.UpdateIndicators(makeIndicators("SPY", 500, 490, 400, 60, 2, 1)) // overwrite

	got := d.GetIndicators("SPY")
	if got.Close != 500 {
		t.Errorf("expected overwritten close=500, got %.2f", got.Close)
	}
}

// ---- Concurrency ----------------------------------------------------------

func TestDetector_ConcurrentReadWrite_NoRace(t *testing.T) {
	d := regime.NewDetector([]string{"SPY"}, nil)

	done := make(chan struct{})

	go func() {
		for i := range 500 {
			d.UpdateIndicators(makeIndicators("SPY", float64(400+i), 390, 350, 55, 2, 1))
		}
		close(done)
	}()

	for range 500 {
		d.AnalyzeSymbol("SPY")
		d.HasSufficientData()
		d.GetMarketContext()
	}

	<-done
}
