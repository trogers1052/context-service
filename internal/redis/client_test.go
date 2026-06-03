package redis

import (
	"errors"
	"net"
	"testing"
)

// ---- isConnectionError ------------------------------------------------------

func TestIsConnectionError_Nil_ReturnsFalse(t *testing.T) {
	if isConnectionError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsConnectionError_NetError_ReturnsTrue(t *testing.T) {
	// A net.OpError satisfies net.Error interface
	netErr := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}
	if !isConnectionError(netErr) {
		t.Error("expected true for net.Error")
	}
}

func TestIsConnectionError_ConnectionRefused_ReturnsTrue(t *testing.T) {
	err := errors.New("dial tcp: connection refused")
	if !isConnectionError(err) {
		t.Error("expected true for 'connection refused' string")
	}
}

func TestIsConnectionError_ConnectionReset_ReturnsTrue(t *testing.T) {
	err := errors.New("read: connection reset by peer")
	if !isConnectionError(err) {
		t.Error("expected true for 'connection reset'")
	}
}

func TestIsConnectionError_BrokenPipe_ReturnsTrue(t *testing.T) {
	err := errors.New("write: broken pipe")
	if !isConnectionError(err) {
		t.Error("expected true for 'broken pipe'")
	}
}

func TestIsConnectionError_IOTimeout_ReturnsTrue(t *testing.T) {
	err := errors.New("read tcp: i/o timeout")
	if !isConnectionError(err) {
		t.Error("expected true for 'i/o timeout'")
	}
}

func TestIsConnectionError_EOF_ReturnsTrue(t *testing.T) {
	err := errors.New("unexpected EOF")
	if !isConnectionError(err) {
		t.Error("expected true for 'EOF'")
	}
}

func TestIsConnectionError_ClosedNetworkConnection_ReturnsTrue(t *testing.T) {
	err := errors.New("use of closed network connection")
	if !isConnectionError(err) {
		t.Error("expected true for 'use of closed network connection'")
	}
}

func TestIsConnectionError_LogicError_ReturnsFalse(t *testing.T) {
	err := errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	if isConnectionError(err) {
		t.Error("expected false for WRONGTYPE logic error")
	}
}

func TestIsConnectionError_GenericError_ReturnsFalse(t *testing.T) {
	err := errors.New("some random error")
	if isConnectionError(err) {
		t.Error("expected false for generic error")
	}
}

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
