package regime_test

import (
	"testing"

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
