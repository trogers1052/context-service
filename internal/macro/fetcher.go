// Package macro fetches macroeconomic risk indicators from the FRED API.
//
// Core series (required — a fetch failure marks all macro signals unavailable):
//
//	VIXCLS         — CBOE Volatility Index (equity fear gauge)
//	BAMLH0A0HYM2   — ICE BofA US High Yield Option-Adjusted Spread (credit stress)
//	BAMLC0A0CM     — ICE BofA US Corporate (Investment Grade) OAS (credit quality)
//
// Peripheral series (best-effort — a failure leaves that field zero but does
// not blank out the core signals above):
//
//	T10Y2Y         — 10Y minus 2Y Treasury spread (yield-curve state)
//	DGS10          — 10Y Treasury yield
//	DTWEXBGS       — Broad trade-weighted US Dollar Index
//	ICSA4WSA       — 4-week moving average of initial jobless claims
//	SAHMREALTIME   — Sahm Rule recession indicator
//	VXVCLS         — CBOE 3-month VIX (VIX3M) for the volatility term structure
//
// FRED updates both series once per business day after market close.
// The Fetcher should be refreshed every 4 hours during market hours so the
// regime detector picks up the latest reading by end-of-day.
//
// When FRED is unavailable or the API key is not configured, Refresh() returns
// an error and Get() returns MacroSignals with Available=false.  All regime
// logic treats unavailable macro signals as a no-op (permissive).
package macro

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	fredBaseURL = "https://api.stlouisfed.org/fred/series/observations"

	// VIX thresholds
	vixLow      = 15.0 // below → LOW (risk-on)
	vixElevated = 25.0 // at or above → ELEVATED
	vixCrisis   = 35.0 // at or above → CRISIS

	// HY spread thresholds (percentage points over Treasury)
	hyTight  = 3.5 // below → TIGHT (risk-on)
	hyWide   = 5.0 // at or above → WIDE
	hyCrisis = 7.0 // at or above → CRISIS

	// IG (investment-grade) spread thresholds (percentage points over Treasury).
	// IG OAS runs far tighter than HY: ~1.3% long-run average, ~0.8% at 2021
	// lows, ~4% at the COVID peak, ~6.5% in 2008.
	igTight  = 1.0 // below → TIGHT (risk-on)
	igWide   = 2.0 // at or above → WIDE
	igCrisis = 3.0 // at or above → CRISIS

	// Yield-curve (10Y−2Y, T10Y2Y) state thresholds in percentage points.
	curveFlatBelow  = 0.5 // [0, 0.5) → FLAT
	curveSteepAtGTE = 1.5 // ≥ 1.5    → STEEP; negative → INVERTED; else NORMAL

	// Sahm Rule triggers a recession signal at or above 0.5.
	sahmTrigger = 0.5

	// VIX term-structure (VIX / VIX3M) state thresholds.
	termBackwardationAt = 1.00 // ≥ 1.00 → BACKWARDATION (acute near-term stress)
	termContangoBelow   = 0.95 // ≤ 0.95 → CONTANGO (calm baseline)
)

// VIX level classifications.
const (
	VIXLevelLow      = "LOW"
	VIXLevelNormal   = "NORMAL"
	VIXLevelElevated = "ELEVATED"
	VIXLevelCrisis   = "CRISIS"
)

// HY spread level classifications.
const (
	HYLevelTight  = "TIGHT"
	HYLevelNormal = "NORMAL"
	HYLevelWide   = "WIDE"
	HYLevelCrisis = "CRISIS"
)

// IG spread level classifications.
const (
	IGLevelTight  = "TIGHT"
	IGLevelNormal = "NORMAL"
	IGLevelWide   = "WIDE"
	IGLevelCrisis = "CRISIS"
)

// Yield-curve state classifications (based on the 10Y−2Y spread).
const (
	CurveInverted = "INVERTED"
	CurveFlat     = "FLAT"
	CurveNormal   = "NORMAL"
	CurveSteep    = "STEEP"
)

// VIX term-structure classifications (VIX vs VIX3M).
const (
	TermContango      = "CONTANGO"
	TermFlat          = "FLAT"
	TermBackwardation = "BACKWARDATION"
)

// MacroSignals holds the most recently fetched macroeconomic risk indicators.
// Available is false when FRED is unreachable or not configured; in that case
// all other fields are zero-valued and regime logic applies no adjustment.
type MacroSignals struct {
	VIX           float64 `json:"vix"`
	VIXLevel      string  `json:"vix_level"`
	HYSpread      float64 `json:"hy_spread"`
	HYLevel       string  `json:"hy_level"`
	IGSpread      float64 `json:"ig_spread"`
	IGLevel       string  `json:"ig_level"`
	QualitySpread float64 `json:"quality_spread"` // HY − IG: junk-vs-quality premium

	// Rates (best-effort; zero when the FRED series is unavailable).
	Ten2Spread float64 `json:"ten2_spread"` // T10Y2Y: 10Y − 2Y Treasury spread
	TenYear    float64 `json:"ten_year"`    // DGS10: 10Y Treasury yield
	CurveState string  `json:"curve_state"` // INVERTED | FLAT | NORMAL | STEEP ("" if unavailable)
	DXY        float64 `json:"dxy"`         // DTWEXBGS: broad trade-weighted dollar

	// Volatility term structure (best-effort).
	VIX3M              float64 `json:"vix_3m"`         // VXVCLS: CBOE 3-month VIX
	TermStructure      float64 `json:"vix_term_ratio"` // VIX / VIX3M
	TermStructureState string  `json:"vix_term_state"` // CONTANGO | FLAT | BACKWARDATION ("" if unavailable)

	// Recession (best-effort).
	JoblessClaims4wk float64 `json:"jobless_claims_4wk"` // ICSA4WSA: 4-wk MA of initial claims
	SahmRule         float64 `json:"sahm_rule"`          // SAHMREALTIME
	SahmTriggered    bool    `json:"sahm_triggered"`     // SahmRule ≥ 0.5

	FetchedAt time.Time `json:"fetched_at"`
	Available bool      `json:"available"`
}

// Fetcher fetches macro risk indicators from the FRED API and caches the
// most recent result.  It is safe for concurrent use.
type Fetcher struct {
	apiKey  string
	baseURL string // overridable for tests
	client  *http.Client
	mu      sync.RWMutex
	current MacroSignals
}

// NewFetcher creates a Fetcher that talks to the live FRED API.
func NewFetcher(apiKey string) *Fetcher {
	return newFetcher(apiKey, fredBaseURL)
}

// NewFetcherWithBaseURL creates a Fetcher pointing at a custom base URL.
// Intended for unit tests that spin up an httptest.Server.
func NewFetcherWithBaseURL(apiKey, baseURL string) *Fetcher {
	return newFetcher(apiKey, baseURL)
}

func newFetcher(apiKey, baseURL string) *Fetcher {
	return &Fetcher{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Get returns the most recently cached macro signals.
// Returns MacroSignals with Available=false if Refresh has never succeeded.
func (f *Fetcher) Get() MacroSignals {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.current
}

// Refresh fetches the latest VIX and HY spread from FRED and updates the
// cached signals.  Returns an error if either series cannot be fetched; in
// that case the previous cached value (if any) is left unchanged.
func (f *Fetcher) Refresh() error {
	vix, err := f.fetchLatestValue("VIXCLS")
	if err != nil {
		return fmt.Errorf("VIXCLS: %w", err)
	}

	hy, err := f.fetchLatestValue("BAMLH0A0HYM2")
	if err != nil {
		return fmt.Errorf("BAMLH0A0HYM2: %w", err)
	}

	ig, err := f.fetchLatestValue("BAMLC0A0CM")
	if err != nil {
		return fmt.Errorf("BAMLC0A0CM: %w", err)
	}

	// Peripheral series are best-effort: a failure must not blank out the core
	// credit/vol signals above.
	ten2, ten2OK := f.fetchOptional("T10Y2Y")
	tenYear, _ := f.fetchOptional("DGS10")
	dxy, _ := f.fetchOptional("DTWEXBGS")
	claims4wk, _ := f.fetchOptional("ICSA4WSA")
	sahm, sahmOK := f.fetchOptional("SAHMREALTIME")
	vix3m, vix3mOK := f.fetchOptional("VXVCLS")

	curveState := ""
	if ten2OK {
		curveState = ClassifyCurve(ten2)
	}

	termRatio := 0.0
	termState := ""
	if vix3mOK && vix3m > 0 {
		termRatio = vix / vix3m
		termState = ClassifyTermStructure(termRatio)
	}

	signals := MacroSignals{
		VIX:                vix,
		VIXLevel:           ClassifyVIX(vix),
		HYSpread:           hy,
		HYLevel:            ClassifyHY(hy),
		IGSpread:           ig,
		IGLevel:            ClassifyIG(ig),
		QualitySpread:      hy - ig,
		Ten2Spread:         ten2,
		TenYear:            tenYear,
		CurveState:         curveState,
		DXY:                dxy,
		VIX3M:              vix3m,
		TermStructure:      termRatio,
		TermStructureState: termState,
		JoblessClaims4wk:   claims4wk,
		SahmRule:           sahm,
		SahmTriggered:      sahmOK && sahm >= sahmTrigger,
		FetchedAt:          time.Now(),
		Available:          true,
	}

	f.mu.Lock()
	f.current = signals
	f.mu.Unlock()

	log.Printf("Macro signals refreshed: VIX=%.2f (%s) HY=%.2f%% (%s) IG=%.2f%% (%s) QS=%.2f%% | 10Y2Y=%.2f (%s) DXY=%.2f Sahm=%.2f VIX3M=%.2f (%s)",
		vix, signals.VIXLevel, hy, signals.HYLevel, ig, signals.IGLevel, signals.QualitySpread,
		signals.Ten2Spread, signals.CurveState, signals.DXY, signals.SahmRule,
		signals.VIX3M, signals.TermStructureState)
	return nil
}

// fredResponse is the relevant subset of the FRED API JSON response.
type fredResponse struct {
	Observations []struct {
		Date  string `json:"date"`
		Value string `json:"value"`
	} `json:"observations"`
}

// fetchLatestValue fetches the most recent non-missing observation for a
// FRED series.  FRED uses "." to denote missing data (weekends, holidays).
func (f *Fetcher) fetchLatestValue(series string) (float64, error) {
	url := fmt.Sprintf(
		"%s?series_id=%s&api_key=%s&file_type=json&sort_order=desc&limit=5",
		f.baseURL, series, f.apiKey,
	)

	resp, err := f.client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("FRED API returned HTTP %d", resp.StatusCode)
	}

	var data fredResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}

	// Skip "." (missing/weekend) entries and return the first valid value.
	for _, obs := range data.Observations {
		if obs.Value == "." {
			continue
		}
		var val float64
		if _, err := fmt.Sscanf(obs.Value, "%f", &val); err == nil {
			return val, nil
		}
	}

	return 0, fmt.Errorf("no valid observation for %s", series)
}

// ClassifyVIX maps a VIX level to a human-readable risk label.
// Exported so the regime and test packages can use it directly.
func ClassifyVIX(vix float64) string {
	switch {
	case vix >= vixCrisis:
		return VIXLevelCrisis
	case vix >= vixElevated:
		return VIXLevelElevated
	case vix < vixLow:
		return VIXLevelLow
	default:
		return VIXLevelNormal
	}
}

// ClassifyHY maps an HY option-adjusted spread (in percentage points) to a
// human-readable credit-stress label.
// Exported so the regime and test packages can use it directly.
func ClassifyHY(spread float64) string {
	switch {
	case spread >= hyCrisis:
		return HYLevelCrisis
	case spread >= hyWide:
		return HYLevelWide
	case spread < hyTight:
		return HYLevelTight
	default:
		return HYLevelNormal
	}
}

// ClassifyIG maps an investment-grade corporate OAS (in percentage points) to a
// human-readable credit-quality label.  IG spreads run far tighter than HY, so
// the thresholds are scaled accordingly.
// Exported so the regime and test packages can use it directly.
func ClassifyIG(spread float64) string {
	switch {
	case spread >= igCrisis:
		return IGLevelCrisis
	case spread >= igWide:
		return IGLevelWide
	case spread < igTight:
		return IGLevelTight
	default:
		return IGLevelNormal
	}
}

// ClassifyCurve maps the 10Y−2Y Treasury spread (percentage points) to a
// yield-curve state.  A negative spread is an inverted curve.
func ClassifyCurve(spread float64) string {
	switch {
	case spread < 0:
		return CurveInverted
	case spread < curveFlatBelow:
		return CurveFlat
	case spread >= curveSteepAtGTE:
		return CurveSteep
	default:
		return CurveNormal
	}
}

// ClassifyTermStructure maps the VIX/VIX3M ratio to a volatility term-structure
// state. Backwardation (ratio ≥ 1) signals acute near-term stress; contango
// (ratio ≤ 0.95) is the calm baseline.
func ClassifyTermStructure(ratio float64) string {
	switch {
	case ratio >= termBackwardationAt:
		return TermBackwardation
	case ratio <= termContangoBelow:
		return TermContango
	default:
		return TermFlat
	}
}

// fetchOptional fetches a FRED series but treats failure as non-fatal: it
// returns (value, true) on success and (0, false) when the series is
// unavailable.  Used for peripheral macro series so that one missing series
// does not blank out the required VIX/HY/IG signals.
func (f *Fetcher) fetchOptional(series string) (float64, bool) {
	v, err := f.fetchLatestValue(series)
	if err != nil {
		log.Printf("macro: optional series %s unavailable: %v", series, err)
		return 0, false
	}
	return v, true
}
