package regime_test

import (
	"math"
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/macro"
	"github.com/trogers1052/context-service/internal/regime"
)

// makeContext returns a minimal MarketContext with the given regime and confidence.
func makeContext(r regime.Regime, confidence float64) *regime.MarketContext {
	return &regime.MarketContext{
		Regime:           r,
		RegimeConfidence: confidence,
		Timestamp:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// makeSignals returns an available MacroSignals with the given VIX and HY level.
func makeSignals(vixLevel, hyLevel string) macro.MacroSignals {
	return macro.MacroSignals{
		VIXLevel:  vixLevel,
		HYLevel:   hyLevel,
		FetchedAt: time.Now(),
		Available: true,
	}
}

// ---- no-op when unavailable ------------------------------------------------

func TestApplyMacroAdjustments_NotAvailable_NoChange(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	signals := macro.MacroSignals{Available: false}

	regime.ApplyMacroAdjustments(ctx, signals)

	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL (should be unchanged)", ctx.Regime)
	}
	if ctx.RegimeConfidence != 0.9 {
		t.Errorf("confidence: got %.2f, want 0.90 (should be unchanged)", ctx.RegimeConfidence)
	}
	if ctx.MacroSignals != nil {
		t.Error("MacroSignals should remain nil when signals are not available")
	}
}

// ---- VIX CRISIS ------------------------------------------------------------

func TestApplyMacroAdjustments_VIXCrisis_BullBecomesBear(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelCrisis, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	// confidence floors at 0.85 — but base was 0.9, so it stays 0.9
	if math.Abs(ctx.RegimeConfidence-0.9) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.9000 (max of 0.90 and 0.85)", ctx.RegimeConfidence)
	}
}

func TestApplyMacroAdjustments_VIXCrisis_LowBaseConfidence_FloorsAt085(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.5)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelCrisis, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	if math.Abs(ctx.RegimeConfidence-0.85) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.8500 (floored from 0.50)", ctx.RegimeConfidence)
	}
}

func TestApplyMacroAdjustments_VIXCrisis_SidewaysBecomesBear(t *testing.T) {
	ctx := makeContext(regime.RegimeSideways, 0.6)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelCrisis, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
}

// ---- VIX ELEVATED ----------------------------------------------------------

func TestApplyMacroAdjustments_VIXElevated_BullBecomesSideways(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelElevated, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS", ctx.Regime)
	}
	want := 0.9 * 0.75
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (0.90 × 0.75)", ctx.RegimeConfidence, want)
	}
}

func TestApplyMacroAdjustments_VIXElevated_BearConfirmedWithBoost(t *testing.T) {
	ctx := makeContext(regime.RegimeBear, 0.8)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelElevated, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR (should stay BEAR)", ctx.Regime)
	}
	want := math.Min(1.0, 0.8*1.1)
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (0.80 × 1.10)", ctx.RegimeConfidence, want)
	}
}

func TestApplyMacroAdjustments_VIXElevated_SidewaysUnchanged(t *testing.T) {
	ctx := makeContext(regime.RegimeSideways, 0.6)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelElevated, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS (should be unchanged)", ctx.Regime)
	}
	if math.Abs(ctx.RegimeConfidence-0.6) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.60 (unchanged for SIDEWAYS + elevated VIX)", ctx.RegimeConfidence)
	}
}

// ---- VIX LOW ---------------------------------------------------------------

func TestApplyMacroAdjustments_VIXLow_BullConfidenceBoosted(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelLow, macro.HYLevelNormal))

	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL", ctx.Regime)
	}
	want := math.Min(1.0, 0.9*1.1)
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (0.90 × 1.10)", ctx.RegimeConfidence, want)
	}
}

func TestApplyMacroAdjustments_VIXLow_BullConfidenceCappedAt1(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.99)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelLow, macro.HYLevelNormal))

	if ctx.RegimeConfidence > 1.0 {
		t.Errorf("confidence: got %.4f, want ≤ 1.0 (should be capped)", ctx.RegimeConfidence)
	}
}

func TestApplyMacroAdjustments_VIXLow_NonBullUnchanged(t *testing.T) {
	for _, r := range []regime.Regime{regime.RegimeBear, regime.RegimeSideways} {
		ctx := makeContext(r, 0.7)
		regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelLow, macro.HYLevelNormal))
		if ctx.Regime != r {
			t.Errorf("regime %s: got %s after VIX LOW, want unchanged", r, ctx.Regime)
		}
		if math.Abs(ctx.RegimeConfidence-0.7) > 0.001 {
			t.Errorf("regime %s: confidence %.4f changed, want 0.70 (unchanged)", r, ctx.RegimeConfidence)
		}
	}
}

// ---- HY CRISIS -------------------------------------------------------------

func TestApplyMacroAdjustments_HYCrisis_BullBecomesBear(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelNormal, macro.HYLevelCrisis))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	// base was 0.9 > 0.85, so stays at 0.9
	if math.Abs(ctx.RegimeConfidence-0.9) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.9000", ctx.RegimeConfidence)
	}
}

func TestApplyMacroAdjustments_HYCrisis_LowBaseConfidence_FloorsAt085(t *testing.T) {
	ctx := makeContext(regime.RegimeSideways, 0.5)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelNormal, macro.HYLevelCrisis))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	if math.Abs(ctx.RegimeConfidence-0.85) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.8500", ctx.RegimeConfidence)
	}
}

// ---- HY WIDE ---------------------------------------------------------------

func TestApplyMacroAdjustments_HYWide_BullBecomesSideways(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelNormal, macro.HYLevelWide))

	if ctx.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS", ctx.Regime)
	}
	want := 0.9 * 0.75
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f", ctx.RegimeConfidence, want)
	}
}

func TestApplyMacroAdjustments_HYWide_BearAndSidewaysUnchanged(t *testing.T) {
	for _, r := range []regime.Regime{regime.RegimeBear, regime.RegimeSideways} {
		ctx := makeContext(r, 0.7)
		regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelNormal, macro.HYLevelWide))
		if ctx.Regime != r {
			t.Errorf("regime %s: got %s after HY WIDE, want unchanged", r, ctx.Regime)
		}
	}
}

// ---- HY TIGHT --------------------------------------------------------------

func TestApplyMacroAdjustments_HYTight_BullConfidenceBoosted(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelNormal, macro.HYLevelTight))

	want := math.Min(1.0, 0.9*1.1)
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f", ctx.RegimeConfidence, want)
	}
}

// ---- Double boost (VIX LOW + HY TIGHT) for BULL ---------------------------

func TestApplyMacroAdjustments_RiskOn_DoubleBullBoost(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.8)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelLow, macro.HYLevelTight))

	// 0.8 * 1.1 (VIX low) * 1.1 (HY tight) = 0.968, capped at 1.0
	want := math.Min(1.0, math.Min(1.0, 0.8*1.1)*1.1)
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (double boost)", ctx.RegimeConfidence, want)
	}
	if ctx.Regime != regime.RegimeBull {
		t.Errorf("regime: got %s, want BULL (both risk-on signals)", ctx.Regime)
	}
}

// ---- Combined: VIX ELEVATED + HY WIDE for BULL (no double-dampen) ---------

func TestApplyMacroAdjustments_VIXElevated_HYWide_BullNOTDoubleDampened(t *testing.T) {
	// VIX ELEVATED converts BULL → SIDEWAYS. HY WIDE then sees SIDEWAYS, not BULL,
	// so it does NOT apply a second 0.75 penalty.
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelElevated, macro.HYLevelWide))

	if ctx.Regime != regime.RegimeSideways {
		t.Errorf("regime: got %s, want SIDEWAYS", ctx.Regime)
	}
	// Only VIX penalty applies: 0.9 * 0.75 = 0.675 (NOT 0.9 * 0.75 * 0.75)
	want := 0.9 * 0.75
	if math.Abs(ctx.RegimeConfidence-want) > 0.001 {
		t.Errorf("confidence: got %.4f, want %.4f (single 0.75 penalty, not double)",
			ctx.RegimeConfidence, want)
	}
}

// ---- VIX CRISIS + HY CRISIS (both fire) ------------------------------------

func TestApplyMacroAdjustments_BothCrisis_BearWithHighConfidence(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	regime.ApplyMacroAdjustments(ctx, makeSignals(macro.VIXLevelCrisis, macro.HYLevelCrisis))

	if ctx.Regime != regime.RegimeBear {
		t.Errorf("regime: got %s, want BEAR", ctx.Regime)
	}
	// Both floor at 0.85; base was 0.9, so stays at 0.9
	if math.Abs(ctx.RegimeConfidence-0.9) > 0.001 {
		t.Errorf("confidence: got %.4f, want 0.9000 (max of 0.9 and 0.85)", ctx.RegimeConfidence)
	}
}

// ---- MacroSignals stored in context ----------------------------------------

func TestApplyMacroAdjustments_SignalsStoredInContext(t *testing.T) {
	ctx := makeContext(regime.RegimeBull, 0.9)
	signals := makeSignals(macro.VIXLevelNormal, macro.HYLevelNormal)
	signals.VIX = 20.0
	signals.HYSpread = 4.0

	regime.ApplyMacroAdjustments(ctx, signals)

	if ctx.MacroSignals == nil {
		t.Fatal("MacroSignals should be set in context after adjustment")
	}
	if ctx.MacroSignals.VIX != 20.0 {
		t.Errorf("MacroSignals.VIX: got %.2f, want 20.00", ctx.MacroSignals.VIX)
	}
}
