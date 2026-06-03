package redis

import (
	"context"
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
