package health_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/trogers1052/context-service/internal/health"
)

func doGet(t *testing.T, h http.Handler) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	return rec.Code
}

func TestProbe_FreshAfterBeat(t *testing.T) {
	p := health.NewProbe(time.Hour)
	if !p.Fresh() {
		t.Error("probe should be fresh immediately after creation")
	}
	if code := doGet(t, p.Handler()); code != http.StatusOK {
		t.Errorf("fresh probe: got %d, want 200", code)
	}
}

func TestProbe_StaleReportsUnhealthy(t *testing.T) {
	p := health.NewProbe(20 * time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	if p.Fresh() {
		t.Error("probe should be stale after maxAge elapses")
	}
	if code := doGet(t, p.Handler()); code != http.StatusServiceUnavailable {
		t.Errorf("stale probe: got %d, want 503", code)
	}
}

func TestProbe_BeatRestoresHealth(t *testing.T) {
	p := health.NewProbe(20 * time.Millisecond)
	time.Sleep(40 * time.Millisecond)
	p.Beat()
	if !p.Fresh() {
		t.Error("probe should be fresh again after Beat")
	}
	if code := doGet(t, p.Handler()); code != http.StatusOK {
		t.Errorf("re-beaten probe: got %d, want 200", code)
	}
}
