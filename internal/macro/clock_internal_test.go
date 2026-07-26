package macro

import (
	"testing"
	"time"

	"github.com/trogers1052/trading-go-commons/clock"
)

func TestNewAppliesProductionDefaults(t *testing.T) {
	f := New(Options{APIKey: "key"})

	if f.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, want the live FRED endpoint %q", f.baseURL, DefaultBaseURL)
	}
	if f.clock == nil {
		t.Fatal("clock = nil, want the real system clock")
	}
	if _, ok := f.clock.(clock.SystemClock); !ok {
		t.Errorf("clock = %T, want SystemClock by default", f.clock)
	}
}

func TestNewHonoursOverrides(t *testing.T) {
	clk := clock.Manual(time.Date(2021, 3, 1, 14, 30, 0, 0, time.UTC))

	f := New(Options{APIKey: "key", BaseURL: "http://stub.local/fred", Clock: clk})

	if f.baseURL != "http://stub.local/fred" {
		t.Errorf("baseURL = %q, want the override", f.baseURL)
	}
	if f.clock != clk {
		t.Error("clock was not the injected one")
	}
}

// The legacy constructors must keep behaving exactly as before, since existing
// callers and tests rely on them.
func TestLegacyConstructorsUnchanged(t *testing.T) {
	if got := NewFetcher("key").baseURL; got != DefaultBaseURL {
		t.Errorf("NewFetcher baseURL = %q, want %q", got, DefaultBaseURL)
	}
	if got := NewFetcherWithBaseURL("key", "http://x").baseURL; got != "http://x" {
		t.Errorf("NewFetcherWithBaseURL baseURL = %q, want http://x", got)
	}
}

func TestNewWithNilClockUsesRealClock(t *testing.T) {
	f := New(Options{APIKey: "key", Clock: nil})

	if _, ok := f.clock.(clock.SystemClock); !ok {
		t.Errorf("clock = %T, want SystemClock when nil is passed", f.clock)
	}
}
