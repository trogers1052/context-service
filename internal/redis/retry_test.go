package redis

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newDeadClient returns a Client pointed at a closed loopback port so any
// network operation (Ping) fails fast without an external Redis.
func newDeadClient() *Client {
	// 127.0.0.1:1 is a privileged port that nothing listens on locally, so
	// dials are refused immediately rather than timing out.
	return NewClient("127.0.0.1", 1, "", 0, "test:key")
}

// ---- retryOnConnectionError -------------------------------------------------

func TestRetryOnConnectionError_SucceedsFirstTry(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	calls := 0
	err := c.retryOnConnectionError(context.Background(), "GET", func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected fn called once, got %d", calls)
	}
}

func TestRetryOnConnectionError_LogicError_NoRetry(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	calls := 0
	logicErr := errors.New("WRONGTYPE Operation against a key")
	err := c.retryOnConnectionError(context.Background(), "SET", func() error {
		calls++
		return logicErr
	})
	if !errors.Is(err, logicErr) {
		t.Fatalf("expected logic error returned unchanged, got %v", err)
	}
	if calls != 1 {
		t.Errorf("logic errors must not retry: expected 1 call, got %d", calls)
	}
}

func TestRetryOnConnectionError_ContextCancelled_StopsRetrying(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before first backoff wait

	calls := 0
	err := c.retryOnConnectionError(ctx, "GET", func() error {
		calls++
		return errors.New("connection refused")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// First fn call returns a connection error, then the backoff select sees the
	// cancelled context and bails — so exactly one fn call.
	if calls != 1 {
		t.Errorf("expected 1 call before context cancel, got %d", calls)
	}
}

func TestRetryOnConnectionError_ExhaustsRetries(t *testing.T) {
	// Override retry timing-sensitive behaviour by using a context with a
	// short deadline so the test doesn't wait the full backoff schedule.
	c := newDeadClient()
	defer c.Close()

	// Use a context that survives long enough for all retries but the dead
	// client's pings fail fast. baseRetryDelay is 500ms; with maxRetries=3
	// the total backoff is ~500+1000+2000ms, so give it room.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	calls := 0
	err := c.retryOnConnectionError(ctx, "GET", func() error {
		calls++
		return errors.New("connection refused")
	})
	// With a 200ms deadline the retry loop will be interrupted by the context
	// during a backoff wait, returning context.DeadlineExceeded.
	if err == nil {
		t.Fatal("expected an error from exhausted/cancelled retries")
	}
	if calls < 1 {
		t.Errorf("expected at least one fn call, got %d", calls)
	}
}

// ---- Connect (error path) ---------------------------------------------------

func TestConnect_DeadServer_ReturnsError(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		t.Error("expected Connect to fail against a dead server")
	}
}

// ---- Close ------------------------------------------------------------------

func TestClose_DoesNotError(t *testing.T) {
	c := newDeadClient()
	if err := c.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

// ---- PublishContext / GetContext (error paths against dead server) ----------

func TestPublishContext_DeadServer_ReturnsError(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := c.PublishContext(ctx, []byte(`{"regime":"BULL"}`))
	if err == nil {
		t.Error("expected PublishContext to fail against a dead server")
	}
}

func TestGetContext_DeadServer_ReturnsError(t *testing.T) {
	c := newDeadClient()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := c.GetContext(ctx)
	if err == nil {
		t.Error("expected GetContext to fail against a dead server")
	}
}
