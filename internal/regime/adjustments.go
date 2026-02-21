package regime

import (
	"math"

	"github.com/trogers1052/context-service/internal/macro"
)

// ApplyMacroAdjustments enriches a MarketContext with macro risk signals
// (VIX volatility index and HY credit spreads) and adjusts the regime
// classification to reflect macro conditions that technical indicators alone
// cannot capture.
//
// Rules applied in order (VIX first, HY second):
//
//	VIX CRISIS  (≥35): Override regime to BEAR regardless of technicals.
//	             Floor confidence at 0.85 so a technically strong BEAR
//	             (confidence already above 0.85) is not accidentally reduced.
//
//	VIX ELEVATED(≥25): Downgrade any remaining BULL to SIDEWAYS (−25%
//	             confidence).  Confirm an existing BEAR with a 10% boost.
//
//	VIX LOW     (<15): Risk-on signal — boost BULL confidence 10%.
//
//	HY CRISIS   (≥7%): Override regime to BEAR, floor confidence at 0.85.
//	             Credit market stress is as reliable a bear signal as VIX.
//
//	HY WIDE     (≥5%): Downgrade any remaining BULL to SIDEWAYS (−25%
//	             confidence).  Runs after VIX, so a VIX-elevated BULL is
//	             already SIDEWAYS and this check is a no-op.
//
//	HY TIGHT    (<3.5%): Risk-on signal — boost BULL confidence 10%.
//
// When signals.Available is false (FRED unavailable or not configured) this
// function is a no-op: the regime and MacroSignals field are left untouched.
// This is intentionally permissive — technical indicators already drove the
// regime; macro unavailability should not artificially dampen confidence.
func ApplyMacroAdjustments(ctx *MarketContext, signals macro.MacroSignals) {
	if !signals.Available {
		return
	}

	ctx.MacroSignals = &signals

	// --- VIX adjustments ------------------------------------------------

	switch signals.VIXLevel {

	case macro.VIXLevelCrisis:
		// Extreme fear: override regime to BEAR.
		// Floor (not replace) confidence so a technically confirmed BEAR
		// at 0.9 is not reduced to 0.85.
		ctx.Regime = RegimeBear
		ctx.RegimeConfidence = math.Max(ctx.RegimeConfidence, 0.85)

	case macro.VIXLevelElevated:
		// Elevated fear: BULL regime is not credible.
		if ctx.Regime == RegimeBull {
			ctx.Regime = RegimeSideways
			ctx.RegimeConfidence *= 0.75
		}
		// Confirms an existing BEAR.
		if ctx.Regime == RegimeBear {
			ctx.RegimeConfidence = math.Min(1.0, ctx.RegimeConfidence*1.1)
		}

	case macro.VIXLevelLow:
		// Risk-on environment: small confidence boost for BULL.
		if ctx.Regime == RegimeBull {
			ctx.RegimeConfidence = math.Min(1.0, ctx.RegimeConfidence*1.1)
		}
	}

	// --- HY spread adjustments ------------------------------------------
	// Runs after VIX so VIX-elevated markets that already became SIDEWAYS
	// are not double-penalised by the HY WIDE check.

	switch signals.HYLevel {

	case macro.HYLevelCrisis:
		// Severe credit stress: override to BEAR.
		ctx.Regime = RegimeBear
		ctx.RegimeConfidence = math.Max(ctx.RegimeConfidence, 0.85)

	case macro.HYLevelWide:
		// Stressed credit: BULL is not credible.
		if ctx.Regime == RegimeBull {
			ctx.Regime = RegimeSideways
			ctx.RegimeConfidence *= 0.75
		}

	case macro.HYLevelTight:
		// Risk-on credit conditions: small boost for BULL.
		if ctx.Regime == RegimeBull {
			ctx.RegimeConfidence = math.Min(1.0, ctx.RegimeConfidence*1.1)
		}
	}
}
