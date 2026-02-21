// Package macro fetches macroeconomic risk indicators from the FRED API.
//
// Two series are tracked daily:
//
//	VIXCLS         — CBOE Volatility Index (equity fear gauge)
//	BAMLH0A0HYM2   — ICE BofA US High Yield Option-Adjusted Spread (credit stress)
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

// MacroSignals holds the most recently fetched macroeconomic risk indicators.
// Available is false when FRED is unreachable or not configured; in that case
// all other fields are zero-valued and regime logic applies no adjustment.
type MacroSignals struct {
	VIX       float64   `json:"vix"`
	VIXLevel  string    `json:"vix_level"`
	HYSpread  float64   `json:"hy_spread"`
	HYLevel   string    `json:"hy_level"`
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

	signals := MacroSignals{
		VIX:       vix,
		VIXLevel:  ClassifyVIX(vix),
		HYSpread:  hy,
		HYLevel:   ClassifyHY(hy),
		FetchedAt: time.Now(),
		Available: true,
	}

	f.mu.Lock()
	f.current = signals
	f.mu.Unlock()

	log.Printf("Macro signals refreshed: VIX=%.2f (%s) HY=%.2f%% (%s)",
		vix, signals.VIXLevel, hy, signals.HYLevel)
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
