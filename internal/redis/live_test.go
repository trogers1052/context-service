package redis

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// miniClient spins up an in-process miniredis and returns a connected Client.
// This exercises the success paths (Connect, PublishContext, GetContext) with
// zero external dependency — no Docker, no always-on Redis.
func miniClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	port, err := strconv.Atoi(mr.Port())
	if err != nil {
		mr.Close()
		t.Fatalf("bad miniredis port %q: %v", mr.Port(), err)
	}
	c := NewClient(mr.Host(), port, "", 0, "cov:redis:context")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		c.Close()
		mr.Close()
		t.Fatalf("Connect to miniredis failed: %v", err)
	}
	return c, mr
}

func TestLive_PublishAndGetContext_RoundTrip(t *testing.T) {
	c, mr := miniClient(t)
	defer mr.Close()
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	payload := []byte(`{"regime":"BULL","regime_confidence":0.9}`)
	if err := c.PublishContext(ctx, payload); err != nil {
		t.Fatalf("PublishContext failed: %v", err)
	}

	got, err := c.GetContext(ctx)
	if err != nil {
		t.Fatalf("GetContext failed: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("round trip mismatch: got %q, want %q", got, payload)
	}
}

func TestLive_GetContext_MissingKey_ReturnsNil(t *testing.T) {
	c, mr := miniClient(t)
	defer mr.Close()
	defer c.Close()

	c.contextKey = "cov:redis:absent:" + strconv.FormatInt(time.Now().UnixNano(), 10)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := c.GetContext(ctx)
	if err != nil {
		t.Fatalf("GetContext on missing key should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing key, got %q", got)
	}
}

func TestLive_Connect_Succeeds(t *testing.T) {
	c, mr := miniClient(t) // Connect already exercised inside miniClient
	defer mr.Close()
	defer c.Close()
}
