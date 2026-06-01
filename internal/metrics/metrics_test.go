package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ---- SetCurrentRegime -------------------------------------------------------

func TestSetCurrentRegime_ActiveSetToOne(t *testing.T) {
	SetCurrentRegime("BULL")
	if got := testutil.ToFloat64(CurrentRegime.WithLabelValues("BULL")); got != 1 {
		t.Errorf("BULL gauge: got %v, want 1", got)
	}
}

func TestSetCurrentRegime_OthersSetToZero(t *testing.T) {
	SetCurrentRegime("BEAR")
	if got := testutil.ToFloat64(CurrentRegime.WithLabelValues("BEAR")); got != 1 {
		t.Errorf("BEAR gauge: got %v, want 1", got)
	}
	for _, r := range []string{"BULL", "SIDEWAYS", "UNKNOWN"} {
		if got := testutil.ToFloat64(CurrentRegime.WithLabelValues(r)); got != 0 {
			t.Errorf("%s gauge: got %v, want 0 when BEAR is active", r, got)
		}
	}
}

func TestSetCurrentRegime_Transition_FlipsGauges(t *testing.T) {
	SetCurrentRegime("BULL")
	SetCurrentRegime("SIDEWAYS")
	if got := testutil.ToFloat64(CurrentRegime.WithLabelValues("BULL")); got != 0 {
		t.Errorf("BULL gauge after transition: got %v, want 0", got)
	}
	if got := testutil.ToFloat64(CurrentRegime.WithLabelValues("SIDEWAYS")); got != 1 {
		t.Errorf("SIDEWAYS gauge after transition: got %v, want 1", got)
	}
}

func TestSetCurrentRegime_UnknownActiveLabel_AllZero(t *testing.T) {
	// An active label not in AllRegimes leaves every known gauge at 0.
	SetCurrentRegime("NOT_A_REAL_REGIME")
	for _, r := range AllRegimes {
		if got := testutil.ToFloat64(CurrentRegime.WithLabelValues(r)); got != 0 {
			t.Errorf("%s gauge: got %v, want 0 for unknown active regime", r, got)
		}
	}
}

func TestAllRegimes_ContainsExpected(t *testing.T) {
	want := map[string]bool{"BULL": true, "BEAR": true, "SIDEWAYS": true, "UNKNOWN": true}
	if len(AllRegimes) != len(want) {
		t.Fatalf("AllRegimes length: got %d, want %d", len(AllRegimes), len(want))
	}
	for _, r := range AllRegimes {
		if !want[r] {
			t.Errorf("unexpected regime in AllRegimes: %q", r)
		}
	}
}

// ---- Metric vars are registered and usable ----------------------------------

func TestCounters_IncrementCleanly(t *testing.T) {
	// These just exercise the metric objects to ensure they are wired up and
	// don't panic on use (e.g. label cardinality / duplicate registration).
	KafkaConsumed.WithLabelValues("SPY").Inc()
	KafkaPublished.WithLabelValues("heartbeat").Inc()
	KafkaPublishErrors.Inc()
	KafkaReconnects.Inc()
	RegimeTransitions.WithLabelValues("BULL", "BEAR").Inc()
	ParseErrors.Inc()
	RedisWriteErrors.Inc()
	FredFetchErrors.Inc()

	if got := testutil.ToFloat64(KafkaConsumed.WithLabelValues("SPY")); got < 1 {
		t.Errorf("KafkaConsumed[SPY]: got %v, want >= 1", got)
	}
}

func TestHistograms_ObserveCleanly(t *testing.T) {
	RedisWriteDuration.Observe(0.01)
	FredFetchDuration.Observe(0.5)
}

func TestGauges_SetCleanly(t *testing.T) {
	VixLevel.Set(18.5)
	if got := testutil.ToFloat64(VixLevel); got != 18.5 {
		t.Errorf("VixLevel: got %v, want 18.5", got)
	}
	SectorStrength.WithLabelValues("XLK").Set(1.25)
	if got := testutil.ToFloat64(SectorStrength.WithLabelValues("XLK")); got != 1.25 {
		t.Errorf("SectorStrength[XLK]: got %v, want 1.25", got)
	}
}
