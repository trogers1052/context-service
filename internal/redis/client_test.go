package redis

import (
	"testing"
)

// ---- NewClient construction -------------------------------------------------

func TestNewClient_SetsContextKey(t *testing.T) {
	c := NewClient("localhost", 6379, "", 0, "test:key")
	if c.contextKey != "test:key" {
		t.Errorf("contextKey: got %q, want %q", c.contextKey, "test:key")
	}
}

func TestNewClient_ClientNotNil(t *testing.T) {
	c := NewClient("localhost", 6379, "", 0, "key")
	if c.client == nil {
		t.Error("expected non-nil redis client")
	}
}
