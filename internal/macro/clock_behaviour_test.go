package macro_test

import (
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/macro"
	"github.com/trogers1052/trading-go-commons/clock"
)

// FetchedAt must come from the clock, not the wall. Consumers treat it as
// provenance for how old the macro reading is; under replay a wall-clock stamp
// would make five-year-old data look like it arrived seconds ago.
func TestRefreshStampsFetchedAtFromClock(t *testing.T) {
	srv := newMockFREDServer("18.50", "4.20", "1.20")
	defer srv.Close()
	simulated := time.Date(2021, 3, 1, 14, 30, 0, 0, time.UTC)

	f := macro.New(macro.Options{
		APIKey:  "key",
		BaseURL: srv.URL,
		Clock:   clock.Manual(simulated),
	})
	if err := f.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	got := f.Get().FetchedAt
	if !got.Equal(simulated) {
		t.Errorf("FetchedAt = %v, want simulated %v", got, simulated)
	}
	if time.Since(got) < 365*24*time.Hour {
		t.Errorf("FetchedAt = %v is near the wall clock — the clock seam is not wired", got)
	}
}

// The real clock must still be the default, so nothing changes in production.
func TestRefreshDefaultsToRealClock(t *testing.T) {
	srv := newMockFREDServer("18.50", "4.20", "1.20")
	defer srv.Close()

	f := macro.NewFetcherWithBaseURL("key", srv.URL)
	if err := f.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if d := time.Since(f.Get().FetchedAt); d > time.Minute || d < -time.Minute {
		t.Errorf("FetchedAt = %v, want within a minute of now", f.Get().FetchedAt)
	}
}
