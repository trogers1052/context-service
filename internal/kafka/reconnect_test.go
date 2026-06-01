package kafka

import (
	"context"
	"testing"
	"time"
)

// TestStart_DeadBroker_HitsErrorThresholdAndReconnects drives the consumer
// against a dead broker long enough for consumeLoop to accumulate read errors,
// return to Start, and enter the reconnect/backoff path. The context then
// cancels during a backoff wait, returning promptly.
//
// This exercises:
//   - consumeLoop's consecutive-error counting + per-error pause
//   - Start's reconnect bookkeeping (close old reader, backoff, new reader)
func TestStart_DeadBroker_HitsErrorThresholdAndReconnects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow reconnect test in -short mode")
	}

	c := NewConsumer([]string{"127.0.0.1:1"}, "test.topic", "test-group", func(_, _ []byte) error { return nil })
	defer c.Close()

	// 14s gives consumeLoop time to hit the 5-error threshold (≈5s of 1s pauses)
	// and Start time to enter at least one reconnect backoff (2s) before cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error from Start against a dead broker")
		}
	case <-time.After(25 * time.Second):
		t.Fatal("Start did not return within the deadline")
	}
}
