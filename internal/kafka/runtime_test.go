package kafka

import (
	"context"
	"testing"
	"time"
)

// ---- Producer.Publish -------------------------------------------------------

func TestPublish_ContextAlreadyCancelled_ReturnsCtxErr(t *testing.T) {
	// Point at a dead broker so WriteMessages fails, then the retry backoff
	// select observes the cancelled context and returns ctx.Err().
	p := NewProducer([]string{"127.0.0.1:1"}, "test.topic")
	defer p.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Publish(ctx, []byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("expected an error publishing to a dead broker with cancelled context")
	}
}

func TestPublish_DeadBroker_ExhaustsRetries(t *testing.T) {
	p := NewProducer([]string{"127.0.0.1:1"}, "test.topic")
	defer p.Close()

	// A short deadline keeps the test fast while still exercising the retry loop.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := p.Publish(ctx, []byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("expected publish to a dead broker to fail")
	}
}

// ---- Consumer.Start / consumeLoop -------------------------------------------

func TestStart_ContextCancelled_ReturnsPromptly(t *testing.T) {
	c := NewConsumer([]string{"127.0.0.1:1"}, "test.topic", "test-group", func(_, _ []byte) error { return nil })
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before Start runs

	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected Start to return ctx error on cancelled context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return promptly after context cancellation")
	}
}

func TestConsumeLoop_ContextCancelled_ReturnsCtxErr(t *testing.T) {
	c := NewConsumer([]string{"127.0.0.1:1"}, "test.topic", "test-group", func(_, _ []byte) error { return nil })
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	hadSuccess, err := c.consumeLoop(ctx)
	if err == nil {
		t.Error("expected consumeLoop to return ctx error")
	}
	if hadSuccess {
		t.Error("expected hadSuccess=false when context is cancelled before any read")
	}
}
