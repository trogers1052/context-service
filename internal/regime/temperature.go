package regime

import (
	"math"
	"time"

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

	// Derivatives (C11) — populated by AttachDerivatives from banked history.
	// Zero/absent until enough history has accumulated.
	EMAFast      float64 `json:"ema_fast,omitempty"`
	EMASlow      float64 `json:"ema_slow,omitempty"`
	Velocity1w   float64 `json:"velocity_1w,omitempty"`
	Velocity4w   float64 `json:"velocity_4w,omitempty"`
	Acceleration float64 `json:"acceleration,omitempty"`
	Direction    string  `json:"direction,omitempty"` // HEATING | COOLING | STABLE
	Percentile   float64 `json:"percentile,omitempty"`
}

// Temperature direction (sign of 1-week velocity, with a deadband).
const (
	DirHeating = "HEATING"
	DirCooling = "COOLING"
	DirStable  = "STABLE"
)

const (
	emaFastSpan      = 3
	emaSlowSpan      = 10
	velocityDeadband = 0.01
)

// TempSample is one capital-temperature reading at a point in time, used to
// compute the derivatives (C11).
type TempSample struct {
	Time  time.Time
	Value float64
}

// AttachDerivatives computes velocity / acceleration / direction / percentile
// from the prior temperature history plus the current value (ct.Value at now)
// and sets them on ct.
//
// The velocity is measured on the EMA-smoothed series (differentiating a raw
// noisy series would just amplify the noise), over time-based 1-week / 4-week
// windows. With too little history it leaves the derivative fields at their
// zero values (omitted from JSON).
func AttachDerivatives(ct *CapitalTemperature, prior []TempSample, now time.Time) {
	if ct == nil {
		return
	}
	series := make([]TempSample, 0, len(prior)+1)
	series = append(series, prior...)
	series = append(series, TempSample{Time: now, Value: ct.Value})
	if len(series) < 2 {
		return
	}

	fast := emaSeries(series, emaFastSpan)
	slow := emaSeries(series, emaSlowSpan)
	last := len(series) - 1
	ct.EMAFast = round4(fast[last])
	ct.EMASlow = round4(slow[last])

	f7 := emaAtOrBefore(series, fast, now.Add(-7*24*time.Hour))
	f14 := emaAtOrBefore(series, fast, now.Add(-14*24*time.Hour))
	f28 := emaAtOrBefore(series, fast, now.Add(-28*24*time.Hour))

	ct.Velocity1w = round4(fast[last] - f7)
	ct.Velocity4w = round4(fast[last] - f28)
	// Second difference over consecutive weekly windows: is the heating/cooling
	// itself building (+) or fading (−)?
	ct.Acceleration = round4((fast[last] - f7) - (f7 - f14))
	ct.Direction = classifyDirection(ct.Velocity1w)
	ct.Percentile = round4(percentileOf(series, ct.Value))
}

// emaSeries returns the EMA-smoothed value sequence (seeded with the first value).
func emaSeries(s []TempSample, span int) []float64 {
	out := make([]float64, len(s))
	if len(s) == 0 {
		return out
	}
	alpha := 2.0 / (float64(span) + 1.0)
	out[0] = s[0].Value
	for i := 1; i < len(s); i++ {
		out[i] = alpha*s[i].Value + (1-alpha)*out[i-1]
	}
	return out
}

// emaAtOrBefore returns the ema value at the latest sample whose time is at or
// before t; if t precedes all samples it returns the earliest ema value.
func emaAtOrBefore(s []TempSample, ema []float64, t time.Time) float64 {
	idx := 0
	for i := range s {
		if s[i].Time.After(t) {
			break
		}
		idx = i
	}
	return ema[idx]
}

func classifyDirection(v float64) string {
	switch {
	case v > velocityDeadband:
		return DirHeating
	case v < -velocityDeadband:
		return DirCooling
	default:
		return DirStable
	}
}

// percentileOf returns the fraction of samples with value <= v (0..1).
func percentileOf(s []TempSample, v float64) float64 {
	if len(s) == 0 {
		return 0
	}
	le := 0
	for i := range s {
		if s[i].Value <= v {
			le++
		}
	}
	return float64(le) / float64(len(s))
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
