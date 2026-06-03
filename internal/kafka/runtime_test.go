package kafka

import (
	"context"
	"testing"
	"time"
)

// ---- Producer.Publish -------------------------------------------------------

func TestPublish_DeadBroker_ReturnsError(t *testing.T) {
	// Point at a dead broker: lazily creating the shared SyncProducer fails
	// (out of brokers), so Publish returns a non-nil error rather than silently
	// dropping the context update.
	p := NewProducer([]string{"127.0.0.1:1"}, "test.topic")
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Publish(ctx, []byte("k"), []byte("v")); err == nil {
		t.Fatal("expected publish to a dead broker to fail")
	}
}

func TestPublish_ContextAlreadyCancelled_ReturnsError(t *testing.T) {
	p := NewProducer([]string{"127.0.0.1:1"}, "test.topic")
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := p.Publish(ctx, []byte("k"), []byte("v")); err == nil {
		t.Fatal("expected an error publishing with a cancelled context to a dead broker")
	}
}

// ---- Consumer.Start ---------------------------------------------------------

func TestStart_DeadBroker_ReturnsError(t *testing.T) {
	// Creating the consumer group against a dead broker fails, so Start returns
	// promptly with a non-nil error instead of blocking.
	c := NewConsumer([]string{"127.0.0.1:1"}, "test.topic", "test-group", func(_, _ []byte) error { return nil })
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected an error from Start against a dead broker")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Start did not return within the deadline")
	}
}
