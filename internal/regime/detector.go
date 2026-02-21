package regime

import (
	"log"
	"sync"
	"time"

	"github.com/trogers1052/context-service/internal/macro"
)

// Regime represents the market regime classification
type Regime string

const (
	RegimeBull     Regime = "BULL"
	RegimeBear     Regime = "BEAR"
	RegimeSideways Regime = "SIDEWAYS"
	RegimeUnknown  Regime = "UNKNOWN"
)

// Indicators represents the technical indicators for a symbol
type Indicators struct {
	Symbol        string    `json:"symbol"`
	Close         float64   `json:"close"`
	SMA20         float64   `json:"sma_20"`
	SMA50         float64   `json:"sma_50"`
	SMA200        float64   `json:"sma_200"`
	RSI14         float64   `json:"rsi_14"`
	MACD          float64   `json:"macd"`
	MACDSignal    float64   `json:"macd_signal"`
	MACDHistogram float64   `json:"macd_histogram"`
	ATR14         float64   `json:"atr_14"`
	Volume        int64     `json:"volume"`
	VolumeSMA20   float64   `json:"volume_sma_20"`
	Timestamp     time.Time `json:"timestamp"`
}

// SymbolRegime represents the regime analysis for a single symbol
type SymbolRegime struct {
	Symbol         string  `json:"symbol"`
	Regime         Regime  `json:"regime"`
	Confidence     float64 `json:"confidence"`
	AboveSMA200    bool    `json:"above_sma_200"`
	RSIBullish     bool    `json:"rsi_bullish"`
	MACDBullish    bool    `json:"macd_bullish"`
	TrendStrength  float64 `json:"trend_strength"` // % above/below SMA200
}

// MarketContext represents the overall market context
type MarketContext struct {
	Regime           Regime                  `json:"regime"`
	RegimeConfidence float64                 `json:"regime_confidence"`
	SPYRegime        *SymbolRegime           `json:"spy_regime,omitempty"`
	QQQRegime        *SymbolRegime           `json:"qqq_regime,omitempty"`
	SectorStrength   map[string]float64      `json:"sector_strength,omitempty"`
	SectorLeaders    []string                `json:"sector_leaders,omitempty"`
	SectorLaggards   []string                `json:"sector_laggards,omitempty"`
	MacroSignals     *macro.MacroSignals     `json:"macro_signals,omitempty"`
	Timestamp        time.Time               `json:"timestamp"`
	UpdatedAt        time.Time               `json:"updated_at"`
}

// Detector analyzes indicators to determine market regime
type Detector struct {
	mu              sync.RWMutex
	symbolIndicators map[string]*Indicators
	regimeSymbols   []string
	sectorSymbols   []string
}

// NewDetector creates a new regime detector
func NewDetector(regimeSymbols, sectorSymbols []string) *Detector {
	return &Detector{
		symbolIndicators: make(map[string]*Indicators),
		regimeSymbols:    regimeSymbols,
		sectorSymbols:    sectorSymbols,
	}
}

// UpdateIndicators updates the indicators for a symbol
func (d *Detector) UpdateIndicators(indicators *Indicators) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.symbolIndicators[indicators.Symbol] = indicators
}

// GetIndicators returns the latest indicators for a symbol
func (d *Detector) GetIndicators(symbol string) *Indicators {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.symbolIndicators[symbol]
}

// AnalyzeSymbol determines the regime for a single symbol
func (d *Detector) AnalyzeSymbol(symbol string) *SymbolRegime {
	d.mu.RLock()
	indicators, exists := d.symbolIndicators[symbol]
	d.mu.RUnlock()

	if !exists || indicators == nil {
		return nil
	}

	// Reject zero-value indicators that indicate missing/incomplete data.
	// SMA200=0 would make any positive close appear "above SMA200" (bullish),
	// and RSI=0 is technically possible but extremely unlikely in practice.
	hasSMA200 := indicators.SMA200 != 0
	hasRSI := indicators.RSI14 != 0

	if !hasSMA200 {
		log.Printf("Warning: %s has SMA200=0 (likely missing data), skipping for regime classification", symbol)
	}
	if !hasRSI {
		log.Printf("Warning: %s has RSI=0 (likely missing data), skipping for regime classification", symbol)
	}

	// If both key indicators are missing, we cannot classify the regime.
	if !hasSMA200 && !hasRSI {
		return &SymbolRegime{
			Symbol:     symbol,
			Regime:     RegimeUnknown,
			Confidence: 0.0,
		}
	}

	// Check each regime condition, only counting indicators with valid data
	macdBullish := indicators.MACD > indicators.MACDSignal

	trendStrength := 0.0
	if hasSMA200 {
		trendStrength = ((indicators.Close - indicators.SMA200) / indicators.SMA200) * 100
	}

	// Count bullish signals only from valid indicators, and track how many
	// indicators actually participated so we can scale confidence correctly.
	bullishCount := 0
	validCount := 0

	if hasSMA200 {
		validCount++
		if indicators.Close > indicators.SMA200 {
			bullishCount++
		}
	}
	if hasRSI {
		validCount++
		if indicators.RSI14 > 50 {
			bullishCount++
		}
	}
	validCount++ // MACD is always available (two values, zero is a valid signal)
	if macdBullish {
		bullishCount++
	}

	var rgm Regime
	var confidence float64

	bullishRatio := float64(bullishCount) / float64(validCount)

	switch {
	case bullishRatio >= 0.8:
		rgm = RegimeBull
		confidence = 0.9
	case bullishRatio >= 0.6:
		aboveSMA200 := hasSMA200 && indicators.Close > indicators.SMA200
		if aboveSMA200 {
			rgm = RegimeBull
			confidence = 0.7
		} else {
			rgm = RegimeSideways
			confidence = 0.6
		}
	case bullishRatio >= 0.3:
		rgm = RegimeSideways
		confidence = 0.5
	default:
		rgm = RegimeBear
		confidence = 0.9
	}

	// Reduce confidence when we're missing indicators
	if validCount < 3 {
		confidence *= float64(validCount) / 3.0
	}

	return &SymbolRegime{
		Symbol:        symbol,
		Regime:        rgm,
		Confidence:    confidence,
		AboveSMA200:   hasSMA200 && indicators.Close > indicators.SMA200,
		RSIBullish:    hasRSI && indicators.RSI14 > 50,
		MACDBullish:   macdBullish,
		TrendStrength: trendStrength,
	}
}

// GetMarketContext computes the overall market context
func (d *Detector) GetMarketContext() *MarketContext {
	now := time.Now()

	ctx := &MarketContext{
		Regime:         RegimeUnknown,
		SectorStrength: make(map[string]float64),
		Timestamp:      now,
		UpdatedAt:      now,
	}

	// Analyze SPY
	spyRegime := d.AnalyzeSymbol("SPY")
	if spyRegime != nil {
		ctx.SPYRegime = spyRegime
	}

	// Analyze QQQ
	qqqRegime := d.AnalyzeSymbol("QQQ")
	if qqqRegime != nil {
		ctx.QQQRegime = qqqRegime
	}

	// Treat UNKNOWN regime the same as missing for combination purposes
	spyUsable := spyRegime != nil && spyRegime.Regime != RegimeUnknown
	qqqUsable := qqqRegime != nil && qqqRegime.Regime != RegimeUnknown

	if spyUsable && qqqUsable {
		ctx.Regime, ctx.RegimeConfidence = d.combineRegimes(spyRegime, qqqRegime)
	} else if spyUsable {
		ctx.Regime = spyRegime.Regime
		ctx.RegimeConfidence = spyRegime.Confidence * 0.8 // Lower confidence without QQQ
	} else if qqqUsable {
		ctx.Regime = qqqRegime.Regime
		ctx.RegimeConfidence = qqqRegime.Confidence * 0.7 // Even lower without SPY
	}

	// Calculate sector strength relative to SPY
	d.calculateSectorStrength(ctx)

	return ctx
}

// combineRegimes combines SPY and QQQ regimes into overall market regime
func (d *Detector) combineRegimes(spy, qqq *SymbolRegime) (Regime, float64) {
	if spy == nil || qqq == nil {
		return RegimeUnknown, 0.0
	}

	// Both agree - high confidence
	if spy.Regime == qqq.Regime {
		avgConfidence := (spy.Confidence + qqq.Confidence) / 2
		return spy.Regime, avgConfidence
	}

	// Disagreement - sideways with lower confidence
	if spy.Regime == RegimeBull && qqq.Regime == RegimeBear {
		return RegimeSideways, 0.5
	}
	if spy.Regime == RegimeBear && qqq.Regime == RegimeBull {
		return RegimeSideways, 0.5
	}

	// One sideways, one directional - lean toward the directional
	if spy.Regime == RegimeSideways {
		return qqq.Regime, qqq.Confidence * 0.7
	}
	if qqq.Regime == RegimeSideways {
		return spy.Regime, spy.Confidence * 0.8 // SPY weighted higher
	}

	return RegimeSideways, 0.5
}

// calculateSectorStrength calculates relative strength of each sector vs SPY
func (d *Detector) calculateSectorStrength(ctx *MarketContext) {
	d.mu.RLock()
	spyIndicators := d.symbolIndicators["SPY"]
	d.mu.RUnlock()

	if spyIndicators == nil || spyIndicators.SMA20 == 0 {
		return
	}

	// SPY trend (price vs SMA20)
	spyTrend := (spyIndicators.Close - spyIndicators.SMA20) / spyIndicators.SMA20

	var leaders, laggards []string

	for _, sector := range d.sectorSymbols {
		d.mu.RLock()
		sectorIndicators := d.symbolIndicators[sector]
		d.mu.RUnlock()

		if sectorIndicators == nil || sectorIndicators.SMA20 == 0 {
			continue
		}

		// Sector trend (price vs SMA20)
		sectorTrend := (sectorIndicators.Close - sectorIndicators.SMA20) / sectorIndicators.SMA20

		// Relative strength = sector trend - SPY trend
		relativeStrength := (sectorTrend - spyTrend) * 100
		ctx.SectorStrength[sector] = relativeStrength

		// Classify as leader or laggard
		if relativeStrength > 1.0 {
			leaders = append(leaders, sector)
		} else if relativeStrength < -1.0 {
			laggards = append(laggards, sector)
		}
	}

	ctx.SectorLeaders = leaders
	ctx.SectorLaggards = laggards
}

// HasSufficientData checks if we have enough data to compute regime
func (d *Detector) HasSufficientData() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Need at least SPY
	spy, hasSPY := d.symbolIndicators["SPY"]
	if !hasSPY || spy == nil {
		return false
	}

	// Need at least one key indicator (SMA200 or RSI) with non-zero data
	return spy.SMA200 > 0 || spy.RSI14 > 0
}
