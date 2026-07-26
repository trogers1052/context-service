package regime

import (
	"sync"
	"testing"
	"time"

	"github.com/trogers1052/trading-go-commons/clock"
)

func bullishIndicators(symbol string) *Indicators {
	return &Indicators{
		Symbol: symbol, Close: 100, SMA20: 95, SMA50: 90, SMA200: 80,
		RSI14: 60, MACD: 1.5, MACDSignal: 1.0,
	}
}

// The context's Timestamp/UpdatedAt is what decision-engine ages off when it
// decides whether market context is stale. Under replay it must carry
// simulated time, or every replayed context looks freshly minted and the
// staleness gate never fires — one of the gates the replay exists to exercise.
func TestGetMarketContextStampsFromClock(t *testing.T) {
	simulated := time.Date(2021, 3, 1, 14, 30, 0, 0, time.UTC)
	d := NewDetectorWithClock([]string{"SPY"}, nil, clock.Manual(simulated))
	d.UpdateIndicators(bullishIndicators("SPY"))

	ctx := d.GetMarketContext()

	if !ctx.Timestamp.Equal(simulated) {
		t.Errorf("Timestamp = %v, want simulated %v", ctx.Timestamp, simulated)
	}
	if !ctx.UpdatedAt.Equal(simulated) {
		t.Errorf("UpdatedAt = %v, want simulated %v", ctx.UpdatedAt, simulated)
	}
	if time.Since(ctx.Timestamp) < 365*24*time.Hour {
		t.Errorf("Timestamp = %v is near the wall clock — the clock seam is not wired", ctx.Timestamp)
	}
}

// NewDetector must behave exactly as it did before the clock existed.
func TestNewDetectorDefaultsToRealClock(t *testing.T) {
	d := NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(bullishIndicators("SPY"))

	ctx := d.GetMarketContext()

	if d := time.Since(ctx.Timestamp); d > time.Minute || d < -time.Minute {
		t.Errorf("Timestamp = %v, want within a minute of now", ctx.Timestamp)
	}
}

func TestNewDetectorWithNilClockFallsBackToReal(t *testing.T) {
	d := NewDetectorWithClock([]string{"SPY"}, nil, nil)
	d.UpdateIndicators(bullishIndicators("SPY"))

	if d := time.Since(d.GetMarketContext().Timestamp); d > time.Minute {
		t.Error("nil clock did not fall back to the real clock")
	}
}

func TestSetClock(t *testing.T) {
	simulated := time.Date(2021, 3, 1, 14, 30, 0, 0, time.UTC)
	d := NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(bullishIndicators("SPY"))

	d.SetClock(clock.Manual(simulated))

	if got := d.GetMarketContext().Timestamp; !got.Equal(simulated) {
		t.Errorf("Timestamp after SetClock = %v, want %v", got, simulated)
	}

	// A nil clock must be ignored rather than panicking on the next read.
	d.SetClock(nil)
	if got := d.GetMarketContext().Timestamp; !got.Equal(simulated) {
		t.Errorf("SetClock(nil) changed the clock to %v", got)
	}
}

// SetClock happens during wiring while other goroutines may already be
// reading; -race asserts the guarded accessor actually guards.
func TestSetClockIsRaceFree(t *testing.T) {
	d := NewDetector([]string{"SPY"}, nil)
	d.UpdateIndicators(bullishIndicators("SPY"))

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for range 25 {
				d.SetClock(clock.Manual(time.Date(2021, 3, 1+i%5, 0, 0, 0, 0, time.UTC)))
			}
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				_ = d.GetMarketContext()
			}
		}()
	}
	wg.Wait()
}
