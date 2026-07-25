// Package history persists market-context snapshots to TimescaleDB and reads
// back the capital-temperature series for derivative computation.
//
// It is permissive by design: when history is disabled (no DSN) or the database
// is unreachable, the Writer is nil and every method is a safe no-op, so a
// missing or failing database never blocks the publish path — the same contract
// the FRED macro fetcher uses.
package history

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TemperaturePoint is one banked capital-temperature reading.
type TemperaturePoint struct {
	Time  time.Time
	Value float64
}

// Writer persists snapshots to a TimescaleDB hypertable and reads back the
// temperature series. A nil *Writer is valid and no-ops on every method.
type Writer struct {
	pool *pgxpool.Pool
}

const schemaDDL = `
CREATE TABLE IF NOT EXISTS market_context_history (
    time        timestamptz      NOT NULL,
    payload     jsonb            NOT NULL,
    temperature double precision
);
CREATE INDEX IF NOT EXISTS market_context_history_time_idx
    ON market_context_history (time DESC);`

// New connects to TimescaleDB using connString and ensures the schema exists.
// Returns (nil, nil) when connString is empty (history disabled) — callers
// treat a nil Writer as "history off".
func New(ctx context.Context, connString string) (*Writer, error) {
	if connString == "" {
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("connect timescale: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping timescale: %w", err)
	}
	w := &Writer{pool: pool}
	if err := w.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return w, nil
}

func (w *Writer) ensureSchema(ctx context.Context) error {
	if _, err := w.pool.Exec(ctx, schemaDDL); err != nil {
		return fmt.Errorf("create history table: %w", err)
	}
	// Promote to a hypertable when TimescaleDB is available. Best-effort: a
	// plain table works fine at our modest volume, so a failure here (missing
	// extension, insufficient privileges, catalog quirk) is logged, not fatal.
	if _, err := w.pool.Exec(ctx,
		`SELECT create_hypertable('market_context_history','time',if_not_exists=>TRUE,migrate_data=>TRUE)`,
	); err != nil {
		log.Printf("history: hypertable promotion skipped (using plain table): %v", err)
	}
	return nil
}

// Write banks one snapshot. temperature may be nil when the gauge is unavailable.
func (w *Writer) Write(ctx context.Context, ts time.Time, payload []byte, temperature *float64) error {
	if w == nil || w.pool == nil {
		return nil
	}
	_, err := w.pool.Exec(ctx,
		`INSERT INTO market_context_history (time, payload, temperature) VALUES ($1,$2,$3)`,
		ts, payload, temperature)
	return err
}

// RecentTemperatures returns non-null temperature points at or after `since`,
// oldest first, capped at limit. Returns nil when the writer is disabled.
func (w *Writer) RecentTemperatures(ctx context.Context, since time.Time, limit int) ([]TemperaturePoint, error) {
	if w == nil || w.pool == nil {
		return nil, nil
	}
	rows, err := w.pool.Query(ctx,
		`SELECT time, temperature FROM market_context_history
		 WHERE temperature IS NOT NULL AND time >= $1
		 ORDER BY time ASC
		 LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pts []TemperaturePoint
	for rows.Next() {
		var p TemperaturePoint
		if err := rows.Scan(&p.Time, &p.Value); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

// Close releases the connection pool. Safe on a nil Writer.
func (w *Writer) Close() {
	if w != nil && w.pool != nil {
		w.pool.Close()
	}
}
