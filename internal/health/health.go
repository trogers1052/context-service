// Package health provides a liveness probe whose /health endpoint fails when the
// service's processing loop stops beating — unlike a plain "the HTTP port
// answers" check, which stays green even when the worker goroutine is dead (the
// failure mode that left analytics-service "healthy" for 40 days while its
// consume loop was gone).
package health

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// Probe tracks a liveness heartbeat. The processing loop calls Beat() as it makes
// progress; the endpoint reports unhealthy once the last beat is older than
// maxAge, so a dead or stuck loop is visible to Docker/orchestration.
type Probe struct {
	last   atomic.Int64 // unix nanos of the last beat
	maxAge time.Duration
}

// NewProbe creates a Probe considered fresh for maxAge after each Beat.
func NewProbe(maxAge time.Duration) *Probe {
	p := &Probe{maxAge: maxAge}
	p.Beat()
	return p
}

// Beat records a heartbeat now.
func (p *Probe) Beat() { p.last.Store(time.Now().UnixNano()) }

// Fresh reports whether the last beat is within maxAge.
func (p *Probe) Fresh() bool {
	return time.Since(time.Unix(0, p.last.Load())) <= p.maxAge
}

// Handler serves /health: 200 while fresh, 503 once the heartbeat goes stale.
func (p *Probe) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if p.Fresh() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("stale: processing loop not beating"))
	})
	return mux
}

// Server runs the probe's /health endpoint with conservative timeouts.
type Server struct{ srv *http.Server }

// NewServer builds a health Server on addr (e.g. ":8080") backed by probe p.
func NewServer(addr string, p *Probe) *Server {
	return &Server{srv: &http.Server{
		Addr:              addr,
		Handler:           p.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}}
}

// Start runs the server in the background; errors (other than a clean close) are
// delivered on the returned channel.
func (s *Server) Start() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	return errCh
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.srv.Shutdown(ctx) }
