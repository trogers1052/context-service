package kafka

import (
	"context"
	"log"
	"sync"

	commonskafka "github.com/trogers1052/trading-go-commons/kafka"
)

// Producer publishes messages to a Kafka topic using the shared, sarama-based
// durable Producer from trading-go-commons.
//
// Behaviour preserved from the previous kafka-go implementation:
//   - same output topic (passed through from config),
//   - same key/value bytes for each publish,
//   - durable acks with bounded retry/backoff. The shared producer waits for all
//     in-sync replicas (sarama.WaitForAll) by default — at least as durable as
//     the previous RequireOne — and retries on transient failures, so market
//     context updates aren't silently lost.
type Producer struct {
	brokers []string
	topic   string

	// prod is the shared producer, created lazily on first Publish so that
	// constructing a Producer never requires a live broker (matching the old
	// kafka-go writer's lazy-connect behaviour and the existing tests).
	mu   sync.Mutex
	prod *commonskafka.Producer
}

// NewProducer creates a new Kafka producer. It does not dial the broker; the
// underlying producer is created on the first Publish call.
func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{
		brokers: brokers,
		topic:   topic,
	}
}

// producer returns the lazily-initialised shared producer, creating it on first
// use.
func (p *Producer) producer() (*commonskafka.Producer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.prod != nil {
		return p.prod, nil
	}

	prod, err := commonskafka.NewProducer(
		p.brokers,
		commonskafka.WithClientID("context-service"),
	)
	if err != nil {
		return nil, err
	}
	p.prod = prod
	return p.prod, nil
}

// Publish sends a message to the configured topic with durable acks. The shared
// producer performs bounded internal retries with backoff; a returned error
// means the write permanently failed and the context update may be lost.
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	prod, err := p.producer()
	if err != nil {
		log.Printf("CRITICAL: Kafka producer init FAILED: %v — context update may be lost", err)
		return err
	}

	if err := prod.Publish(ctx, p.topic, key, value); err != nil {
		log.Printf("CRITICAL: Kafka publish FAILED: %v — context update may be lost", err)
		return err
	}
	return nil
}

// Close closes the underlying producer. It is safe to call before any Publish
// (no-op) and is idempotent.
func (p *Producer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.prod == nil {
		return nil
	}
	err := p.prod.Close()
	p.prod = nil
	return err
}
