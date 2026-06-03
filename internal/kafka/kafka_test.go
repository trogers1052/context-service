package kafka

import (
	"testing"
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
}

func TestNewConsumer_ConfigStored(t *testing.T) {
	c := NewConsumer([]string{"broker1:9092", "broker2:9092"}, "my.topic", "my-group", func(_, _ []byte) error { return nil })

	if c.topic != "my.topic" {
		t.Errorf("topic: got %q, want %q", c.topic, "my.topic")
	}
	if c.groupID != "my-group" {
		t.Errorf("groupID: got %q, want %q", c.groupID, "my-group")
	}
	if len(c.brokers) != 2 {
		t.Errorf("brokers: got %d, want 2", len(c.brokers))
	}
}

func TestNewConsumer_HandlerAdaptsKeyValue(t *testing.T) {
	var gotKey, gotVal []byte
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(k, v []byte) error {
		gotKey, gotVal = k, v
		return nil
	})
	if err := c.handler([]byte("key"), []byte("value")); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if string(gotKey) != "key" || string(gotVal) != "value" {
		t.Errorf("handler got key=%q value=%q, want key/value", gotKey, gotVal)
	}
}

// ---- Close ------------------------------------------------------------------

func TestConsumer_Close_BeforeStart_NoError(t *testing.T) {
	c := NewConsumer([]string{"localhost:9092"}, "t", "g", func(_, _ []byte) error { return nil })
	// Close before Start (no consumer group created yet) must be a safe no-op.
	if err := c.Close(); err != nil {
		t.Errorf("Close before Start returned error: %v", err)
	}
}

// ---- NewProducer construction -----------------------------------------------

func TestNewProducer_StoresTopic(t *testing.T) {
	p := NewProducer([]string{"localhost:9092"}, "test.topic")
	if p.topic != "test.topic" {
		t.Errorf("topic: got %q, want %q", p.topic, "test.topic")
	}
	if len(p.brokers) != 1 {
		t.Errorf("brokers: got %d, want 1", len(p.brokers))
	}
}

func TestProducer_Close_BeforePublish_NoError(t *testing.T) {
	p := NewProducer([]string{"localhost:9092"}, "test.topic")
	// Close before any Publish (no underlying producer created) must be a no-op.
	if err := p.Close(); err != nil {
		t.Errorf("Close before Publish returned error: %v", err)
	}
}
