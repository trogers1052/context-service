package kafka

import (
	"context"
	"log"

	commonskafka "github.com/trogers1052/trading-go-commons/kafka"
)

// MessageHandler is called for each consumed message.
type MessageHandler func(key, value []byte) error

// Consumer consumes messages from a Kafka topic using the shared, kafka-go-based
// consumer-group runner from trading-go-commons. It preserves the previous
// behaviour:
//   - same consumer group ID and topic (passed through from config),
//   - the same initial-offset semantics: only NEW messages are read on a fresh
//     group (library-neutral commonskafka.OffsetNewest, which maps onto
//     kafka-go's StartOffset: LastOffset),
//   - commit-after-success handling (the shared runner marks a message once the
//     handler returns; on handler error it logs and continues, matching the old
//     reader's "don't block the consumer" behaviour),
//   - automatic reconnect on session/rebalance errors and graceful shutdown on
//     context cancellation (owned by the shared ConsumerGroup.Run).
type Consumer struct {
	brokers []string
	topic   string
	groupID string
	handler MessageHandler

	// cg is the shared consumer-group runner. It is created lazily in Start so
	// that constructing a Consumer never requires a live broker (preserving the
	// old kafka-go constructor's lazy-dial behaviour and the existing tests that
	// build a Consumer against a dead broker without error).
	cg *commonskafka.ConsumerGroup
}

// NewConsumer creates a new Kafka consumer. It does not dial the broker; the
// underlying consumer group is created when Start is called.
func NewConsumer(brokers []string, topic, groupID string, handler MessageHandler) *Consumer {
	return &Consumer{
		brokers: brokers,
		topic:   topic,
		groupID: groupID,
		handler: handler,
	}
}

// Start begins consuming messages and blocks until ctx is cancelled, then shuts
// down gracefully and returns. Session/rebalance errors are handled internally
// by the shared runner (logged + reconnect with backoff). It returns an error
// if the consumer group cannot be created, or the context's error on shutdown.
//
// Behaviour preserved across the kafka-go migration: the shared
// ConsumerGroup.Run returns nil on context cancellation (graceful shutdown),
// whereas the previous runner surfaced ctx.Err(). To keep this wrapper's
// observable contract identical, Start re-surfaces ctx.Err() when Run returns
// without an error but the context has been cancelled.
//
// The handler is adapted from the shared *kafka.Message into the existing
// (key, value) MessageHandler so the service's handleMessage logic is reused
// byte-for-byte.
func (c *Consumer) Start(ctx context.Context) error {
	log.Printf("Starting Kafka consumer for topic: %s", c.topic)

	cg, err := commonskafka.NewConsumerGroup(
		c.brokers,
		c.groupID,
		[]string{c.topic},
		func(_ context.Context, msg *commonskafka.Message) error {
			return c.handler(msg.Key, msg.Value)
		},
		// Preserve the previous StartOffset: kafka.LastOffset semantics — a fresh
		// consumer group reads only NEW messages, not the historical backlog.
		commonskafka.WithInitialOffset(commonskafka.OffsetNewest),
		commonskafka.WithConsumerClientID("context-service"),
	)
	if err != nil {
		return err
	}
	c.cg = cg

	if runErr := c.cg.Run(ctx); runErr != nil {
		return runErr
	}
	// Run returned nil. If that was due to context cancellation, surface the
	// context error to preserve the pre-migration contract.
	return ctx.Err()
}

// Close releases the underlying consumer group. It is safe to call before Start
// (no-op) and after a graceful Run shutdown.
func (c *Consumer) Close() error {
	if c.cg == nil {
		return nil
	}
	return c.cg.Close()
}
