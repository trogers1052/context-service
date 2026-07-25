// Package sentiment provides market-sentiment gauges for the context service.
//
// This is currently a STUB: the wiring exists end-to-end (types, a Fetcher
// interface, and a MarketContext field) but no real external source is
// connected yet, so StubFetcher reports sentiment as unavailable. Consumers
// treat unavailable sentiment as a no-op, exactly like macro signals when FRED
// is down.
//
// Real sources, in value order (see StubFetcher.Fetch TODOs):
//   - CNN Fear & Greed  — a composite that already subsumes put/call. HIGH value.
//   - CBOE put/call      — low marginal value (inside Fear & Greed). LOW.
//   - AAII bull/bear     — weekly, membership-gated, no clean free API. LOW.
package sentiment

// Signals holds market-sentiment gauges. Available is false until a real source
// is wired; when false all other fields are zero and consumers skip sentiment.
type Signals struct {
	FearGreed      float64 `json:"fear_greed"`       // 0..100 (CNN); 0 = extreme fear, 100 = extreme greed
	FearGreedLabel string  `json:"fear_greed_label"` // EXTREME_FEAR .. EXTREME_GREED
	PutCall        float64 `json:"put_call"`         // CBOE equity put/call ratio
	AAIIBullBear   float64 `json:"aaii_bull_bear"`   // AAII bull-minus-bear spread (percentage points)
	Source         string  `json:"source"`           // which fetcher populated this ("" when none)
	Available      bool    `json:"available"`
}

// Fetcher is the interface a real sentiment source implements. Keeping consumers
// behind this interface lets a real fetcher replace the stub without any change
// to the service or the market:context contract.
type Fetcher interface {
	Fetch() (Signals, error)
}

// StubFetcher is a permissive placeholder that always reports sentiment as
// unavailable. Swap in a real fetcher (see Fetch TODOs) later.
type StubFetcher struct{}

// NewStubFetcher returns a StubFetcher.
func NewStubFetcher() *StubFetcher { return &StubFetcher{} }

// Fetch returns unavailable sentiment.
//
// TODO(C7): implement real sources, highest value first —
//   - CNN Fear & Greed: GET production.dataviz.cnn.io/index/fearandgreed/graphdata
//     (unofficial JSON; parse fear_and_greed.score). Populate FearGreed +
//     FearGreedLabel via ClassifyFearGreed, Source="cnn", Available=true.
//   - CBOE put/call and AAII: only if a stable free source materialises; both
//     are low marginal value for a small account today.
func (StubFetcher) Fetch() (Signals, error) {
	return Signals{Available: false}, nil
}

// Fear & Greed label bands (CNN convention).
const (
	LabelExtremeFear  = "EXTREME_FEAR"
	LabelFear         = "FEAR"
	LabelNeutral      = "NEUTRAL"
	LabelGreed        = "GREED"
	LabelExtremeGreed = "EXTREME_GREED"
)

// ClassifyFearGreed maps a 0..100 score to CNN's label bands.
func ClassifyFearGreed(v float64) string {
	switch {
	case v >= 75:
		return LabelExtremeGreed
	case v >= 55:
		return LabelGreed
	case v > 45:
		return LabelNeutral
	case v > 25:
		return LabelFear
	default:
		return LabelExtremeFear
	}
}
