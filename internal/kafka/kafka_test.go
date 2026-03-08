package kafka

import (
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// ---- NewConsumer construction -----------------------------------------------

func TestNewConsumer_SetsHandler(t *testing.T) {
	handler := func(key, value []byte) error {
		return nil
	}
	c := NewConsumer([]string{"localhost:9092"}, "test.topic", "test-group", handler)
	if c.handler == nil {
		t.Error("expected non-nil handler")
	}
	if c.reader == nil {
		t.Error("expected non-nil reader")
	}
}

func TestNewConsumer_ConfigStored(t *testing.T) {
	c := NewConsumer([]string{"broker1:9092", "broker2:9092"}, "my.topic", "my-group", func(_, _ []byte) error { return nil })

	if c.config.Topic != "my.topic" {
		t.Errorf("config.Topic: got %q, want %q", c.config.Topic, "my.topic")
	}
	if c.config.GroupID != "my-group" {
		t.Errorf("config.GroupID: got %q, want %q", c.config.GroupID, "my-group")
	}
	if len(c.config.Brokers) != 2 {
		t.Errorf("config.Brokers: got %d, want 2", len(c.config.Brokers))
	}
}

func TestNewConsumer_ReaderConfig_MaxWait(t *testing.T) {
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(_, _ []byte) error { return nil })
	if c.config.MaxWait != 500*time.Millisecond {
		t.Errorf("MaxWait: got %v, want 500ms", c.config.MaxWait)
	}
}

func TestNewConsumer_StartsAtLastOffset(t *testing.T) {
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(_, _ []byte) error { return nil })
	if c.config.StartOffset != kafka.LastOffset {
		t.Errorf("StartOffset: got %d, want LastOffset (%d)", c.config.StartOffset, kafka.LastOffset)
	}
}

func TestNewConsumer_NewReader_CreatesFreshReader(t *testing.T) {
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(_, _ []byte) error { return nil })
	original := c.reader
	fresh := c.newReader()
	if fresh == nil {
		t.Fatal("newReader returned nil")
	}
	if fresh == original {
		t.Error("newReader should return a different reader instance")
	}
	// Clean up
	original.Close()
	fresh.Close()
}

// ---- Close ------------------------------------------------------------------

func TestConsumer_Close_DoesNotPanic(t *testing.T) {
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(_, _ []byte) error { return nil })
	// Close should not panic even without a live Kafka
	if err := c.Close(); err != nil {
		// The reader may return an error if it can't reach Kafka, which is fine
		t.Logf("Close returned (expected): %v", err)
	}
}

// ---- NewProducer construction -----------------------------------------------

func TestNewProducer_WriterNotNil(t *testing.T) {
	p := NewProducer([]string{"localhost:9092"}, "test.topic")
	if p.writer == nil {
		t.Error("expected non-nil writer")
	}
}

func TestProducer_Close_DoesNotPanic(t *testing.T) {
	p := NewProducer([]string{"localhost:9092"}, "test.topic")
	// Close should not panic
	if err := p.Close(); err != nil {
		t.Logf("Close returned (expected): %v", err)
	}
}
