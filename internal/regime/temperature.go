package regime

import (
	"math"

	"github.com/trogers1052/context-service/internal/macro"
)

// CapitalTemperature is a 0..1 market-risk gauge (0 = risk-on/calm, 1 =
// risk-off/stressed) synthesised from the sensor-plane signals.
//
// C10 populates Value + Components. The velocity / acceleration / percentile
// derivatives are a deliberate follow-up (C11) that reads the temperature's
// own banked history, so they are intentionally absent here.
type CapitalTemperature struct {
	Value      float64            `json:"value"`                // 0..1
	Components map[string]float64 `json:"components,omitempty"` // per-signal sub-score (0..1)
	Inputs     int                `json:"inputs"`               // signals that contributed
}

// temperatureWeights weights each sub-signal's contribution. Credit and the
// primary trend/breadth signals dominate, mirroring the corpus emphasis
// ("the only thing that matters is credit spreads"). Weights sum to 1.0; the
// computation renormalises over whichever inputs are actually present.
var temperatureWeights = map[string]float64{
	"credit_hy":   0.20,
	"credit_ig":   0.05,
	"vix":         0.15,
	"curve":       0.10,
	"recession":   0.10,
	"sp500_trend": 0.20,
	"breadth":     0.20,
}

// ComputeCapitalTemperature synthesises a 0..1 risk gauge from whatever sensor
// signals are present on ctx, renormalising by the weight of available inputs
// so a missing signal neither helps nor hurts. Returns nil if nothing is set.
//
// It must be called AFTER ApplyMacroAdjustments (which populates ctx.MacroSignals)
// and after GetMarketContext (which sets SP500Trend and Breadth).
func ComputeCapitalTemperature(ctx *MarketContext) *CapitalTemperature {
	comps := map[string]float64{}

	if m := ctx.MacroSignals; m != nil && m.Available {
		if m.HYLevel != "" {
			comps["credit_hy"] = creditScore(m.HYLevel)
		}
		if m.IGLevel != "" {
			comps["credit_ig"] = creditScore(m.IGLevel)
		}
		if m.VIXLevel != "" {
			comps["vix"] = vixScore(m.VIXLevel)
		}
		if m.CurveState != "" {
			comps["curve"] = curveScore(m.CurveState)
		}
		// Include recession only when we actually have a Sahm reading (>0);
		// an exact 0.0 is treated as "no data" and renormalised away.
		if m.SahmTriggered || m.SahmRule > 0 {
			comps["recession"] = recessionScore(m)
		}
	}

	if ctx.SP500Trend != nil {
		comps["sp500_trend"] = trendScore(ctx.SP500Trend)
	}
	if ctx.Breadth != nil {
		comps["breadth"] = breadthScore(ctx.Breadth)
	}

	if len(comps) == 0 {
		return nil
	}

	var weighted, totalWeight float64
	for name, score := range comps {
		w := temperatureWeights[name]
		weighted += w * score
		totalWeight += w
	}

	value := 0.0
	if totalWeight > 0 {
		value = weighted / totalWeight
	}

	return &CapitalTemperature{
		Value:      round4(value),
		Components: roundComponents(comps),
		Inputs:     len(comps),
	}
}

// creditScore maps a credit-spread level (shared HY/IG label set) to risk.
func creditScore(level string) float64 {
	switch level {
	case macro.HYLevelTight: // "TIGHT" (== IGLevelTight)
		return 0.0
	case macro.HYLevelWide: // "WIDE"
		return 0.66
	case macro.HYLevelCrisis: // "CRISIS"
		return 1.0
	default: // "NORMAL"
		return 0.33
	}
}

func vixScore(level string) float64 {
	switch level {
	case macro.VIXLevelLow:
		return 0.0
	case macro.VIXLevelElevated:
		return 0.66
	case macro.VIXLevelCrisis:
		return 1.0
	default: // NORMAL
		return 0.33
	}
}

func curveScore(state string) float64 {
	switch state {
	case macro.CurveSteep:
		return 0.0
	case macro.CurveFlat:
		return 0.6
	case macro.CurveInverted:
		return 1.0
	default: // NORMAL
		return 0.25
	}
}

// recessionScore is 1.0 once the Sahm Rule triggers; below the 0.5 trigger it
// scales linearly toward it.
func recessionScore(m *macro.MacroSignals) float64 {
	if m.SahmTriggered {
		return 1.0
	}
	s := m.SahmRule / 0.5 // 0.5 is the Sahm trigger threshold
	return clamp01(s)
}

// trendScore rewards a clean uptrend and penalises a broken 200-day trend.
func trendScore(t *SP500Trend) float64 {
	switch {
	case t.Above200 && t.CrossState == CrossGolden:
		return 0.0 // healthy uptrend
	case !t.Above200 && t.CrossState == CrossDeath:
		return 1.0 // fully broken
	case !t.Above200:
		return 0.75 // below the 200-day is the primary risk trigger
	default:
		return 0.4 // above 200 but not a clean golden cross
	}
}

// breadthScore turns participation into risk: 100% above the 200-day → 0,
// 0% above → 1.
func breadthScore(b *Breadth) float64 {
	return 1.0 - clamp01(b.PctAbove200/100.0)
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func roundComponents(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = round4(v)
	}
	return out
}
