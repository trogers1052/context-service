package regime_test

import (
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/macro"
	"github.com/trogers1052/context-service/internal/regime"
)

func TestComputeCapitalTemperature_NilWhenNoSignals(t *testing.T) {
	if ct := regime.ComputeCapitalTemperature(&regime.MarketContext{}); ct != nil {
		t.Errorf("expected nil temperature with no signals, got %+v", ct)
	}
}

func TestComputeCapitalTemperature_AllRiskOff_HighValue(t *testing.T) {
	ctx := &regime.MarketContext{
		MacroSignals: &macro.MacroSignals{
			Available:     true,
			HYLevel:       macro.HYLevelCrisis,
			IGLevel:       macro.IGLevelCrisis,
			VIXLevel:      macro.VIXLevelCrisis,
			CurveState:    macro.CurveInverted,
			SahmRule:      0.7,
			SahmTriggered: true,
		},
		SP500Trend: &regime.SP500Trend{Above200: false, CrossState: regime.CrossDeath},
		Breadth:    &regime.Breadth{PctAbove200: 5, Count: 20, Universe: 20},
	}
	ct := regime.ComputeCapitalTemperature(ctx)
	if ct == nil {
		t.Fatal("expected temperature")
	}
	if ct.Value < 0.9 {
		t.Errorf("all-risk-off temperature should be near 1.0, got %.3f", ct.Value)
	}
	if ct.Inputs != 7 {
		t.Errorf("expected 7 contributing signals, got %d", ct.Inputs)
	}
}

func TestComputeCapitalTemperature_AllRiskOn_LowValue(t *testing.T) {
	ctx := &regime.MarketContext{
		MacroSignals: &macro.MacroSignals{
			Available:  true,
			HYLevel:    macro.HYLevelTight,
			IGLevel:    macro.IGLevelTight,
			VIXLevel:   macro.VIXLevelLow,
			CurveState: macro.CurveSteep,
			SahmRule:   0.0, // skipped (no data / healthy)
		},
		SP500Trend: &regime.SP500Trend{Above200: true, CrossState: regime.CrossGolden},
		Breadth:    &regime.Breadth{PctAbove200: 95, Count: 20, Universe: 20},
	}
	ct := regime.ComputeCapitalTemperature(ctx)
	if ct == nil {
		t.Fatal("expected temperature")
	}
	if ct.Value > 0.1 {
		t.Errorf("all-risk-on temperature should be near 0.0, got %.3f", ct.Value)
	}
}

func TestComputeCapitalTemperature_RenormalisesMissingInputs(t *testing.T) {
	// Only breadth present → value equals the breadth sub-score alone.
	ctx := &regime.MarketContext{
		Breadth: &regime.Breadth{PctAbove200: 40, Count: 10, Universe: 10},
	}
	ct := regime.ComputeCapitalTemperature(ctx)
	if ct == nil {
		t.Fatal("expected temperature from breadth alone")
	}
	if ct.Inputs != 1 {
		t.Errorf("expected 1 input, got %d", ct.Inputs)
	}
	// breadthScore = 1 - 40/100 = 0.6; renormalised over its own weight → 0.6
	if ct.Value < 0.59 || ct.Value > 0.61 {
		t.Errorf("expected ~0.60 from breadth alone, got %.3f", ct.Value)
	}
}

func TestComputeCapitalTemperature_UnavailableMacro_UsesTrendBreadthOnly(t *testing.T) {
	ctx := &regime.MarketContext{
		MacroSignals: &macro.MacroSignals{Available: false, HYLevel: macro.HYLevelCrisis},
		SP500Trend:   &regime.SP500Trend{Above200: true, CrossState: regime.CrossGolden},
		Breadth:      &regime.Breadth{PctAbove200: 80, Count: 10, Universe: 10},
	}
	ct := regime.ComputeCapitalTemperature(ctx)
	if ct == nil {
		t.Fatal("expected temperature")
	}
	if ct.Inputs != 2 {
		t.Errorf("unavailable macro must contribute nothing: expected 2 inputs, got %d", ct.Inputs)
	}
}

// ---- AttachDerivatives (C11) ----------------------------------------------

func TestAttachDerivatives_TooFewPoints_NoOp(t *testing.T) {
	ct := &regime.CapitalTemperature{Value: 0.5}
	regime.AttachDerivatives(ct, nil, time.Now())
	if ct.Direction != "" || ct.Velocity1w != 0 {
		t.Errorf("with no history, derivatives should be unset, got %+v", ct)
	}
}

func TestAttachDerivatives_Heating(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	prior := []regime.TempSample{
		{Time: now.Add(-28 * 24 * time.Hour), Value: 0.30},
		{Time: now.Add(-21 * 24 * time.Hour), Value: 0.35},
		{Time: now.Add(-14 * 24 * time.Hour), Value: 0.45},
		{Time: now.Add(-7 * 24 * time.Hour), Value: 0.60},
	}
	ct := &regime.CapitalTemperature{Value: 0.75}
	regime.AttachDerivatives(ct, prior, now)

	if ct.Direction != regime.DirHeating {
		t.Errorf("Direction: got %s, want HEATING", ct.Direction)
	}
	if ct.Velocity1w <= 0 {
		t.Errorf("Velocity1w should be positive when heating, got %.4f", ct.Velocity1w)
	}
	if ct.Velocity4w <= 0 {
		t.Errorf("Velocity4w should be positive, got %.4f", ct.Velocity4w)
	}
	if ct.Percentile < 0.99 { // current 0.75 is the series max → 5/5
		t.Errorf("Percentile: got %.2f, want ~1.0 (current is the max)", ct.Percentile)
	}
}

func TestAttachDerivatives_Cooling(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	prior := []regime.TempSample{
		{Time: now.Add(-28 * 24 * time.Hour), Value: 0.80},
		{Time: now.Add(-21 * 24 * time.Hour), Value: 0.70},
		{Time: now.Add(-14 * 24 * time.Hour), Value: 0.60},
		{Time: now.Add(-7 * 24 * time.Hour), Value: 0.45},
	}
	ct := &regime.CapitalTemperature{Value: 0.30}
	regime.AttachDerivatives(ct, prior, now)

	if ct.Direction != regime.DirCooling {
		t.Errorf("Direction: got %s, want COOLING", ct.Direction)
	}
	if ct.Velocity1w >= 0 {
		t.Errorf("Velocity1w should be negative when cooling, got %.4f", ct.Velocity1w)
	}
}

func TestAttachDerivatives_Stable(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	prior := []regime.TempSample{
		{Time: now.Add(-14 * 24 * time.Hour), Value: 0.50},
		{Time: now.Add(-7 * 24 * time.Hour), Value: 0.50},
	}
	ct := &regime.CapitalTemperature{Value: 0.50}
	regime.AttachDerivatives(ct, prior, now)

	if ct.Direction != regime.DirStable {
		t.Errorf("Direction: got %s, want STABLE for a flat series", ct.Direction)
	}
}
