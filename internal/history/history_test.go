package history_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trogers1052/context-service/internal/history"
)

func TestNew_EmptyDSN_ReturnsNilWriter(t *testing.T) {
	w, err := history.New(context.Background(), "")
	if err != nil {
		t.Fatalf(`New("") should not error, got %v`, err)
	}
	if w != nil {
		t.Error(`New("") should return a nil Writer (history disabled)`)
	}
}

func TestNilWriter_MethodsAreNoOps(t *testing.T) {
	var w *history.Writer // nil — history disabled
	if err := w.Write(context.Background(), time.Now(), []byte(`{}`), nil); err != nil {
		t.Errorf("nil Write should be a no-op, got %v", err)
	}
	pts, err := w.RecentTemperatures(context.Background(), time.Now(), 10)
	if err != nil {
		t.Errorf("nil RecentTemperatures should be a no-op, got %v", err)
	}
	if pts != nil {
		t.Errorf("nil RecentTemperatures should return nil, got %v", pts)
	}
	w.Close() // must not panic
}

// Integration test — runs only when CONTEXT_IT_TIMESCALE_DSN is set. Uses a
// far-past marker window (year 1990) so its rows never collide with real data,
// and deletes them afterward.
func TestWriteAndReadBack_Integration(t *testing.T) {
	dsn := os.Getenv("CONTEXT_IT_TIMESCALE_DSN")
	if dsn == "" {
		t.Skip("set CONTEXT_IT_TIMESCALE_DSN to run the TimescaleDB integration test")
	}
	ctx := context.Background()

	w, err := history.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer w.Close()

	// Cleanup helper (direct pool) — removes only the marker-window rows.
	cleanup := func() {
		if pool, err := pgxpool.New(ctx, dsn); err == nil {
			defer pool.Close()
			pool.Exec(ctx, `DELETE FROM market_context_history WHERE time < '2000-01-01'`)
		}
	}
	cleanup()
	defer cleanup()

	base := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	t1, t2 := 0.42, 0.55
	if err := w.Write(ctx, base, []byte(`{"_it":true}`), &t1); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if err := w.Write(ctx, base.Add(time.Hour), []byte(`{"_it":true}`), &t2); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	pts, err := w.RecentTemperatures(ctx, base.Add(-time.Hour), 100)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("expected exactly 2 marker points, got %d", len(pts))
	}
	if pts[0].Value != t1 { // oldest first
		t.Errorf("first value: got %.2f, want %.2f", pts[0].Value, t1)
	}
	if pts[1].Value != t2 {
		t.Errorf("second value: got %.2f, want %.2f", pts[1].Value, t2)
	}
}
