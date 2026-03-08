package kafka

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	// publishMaxRetries is the number of times to retry a failed Kafka publish.
	// Market context updates are critical for regime-aware trading decisions —
	// a missed publish leaves the decision-engine on stale regime data.
	publishMaxRetries = 3
	publishInitialBackoff = 100 * time.Millisecond
)

// Producer publishes messages to a Kafka topic
type Producer struct {
	writer *kafka.Writer
}

// NewProducer creates a new Kafka producer
func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1,
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
		WriteTimeout: 10 * time.Second,
		Transport: &kafka.Transport{
			DialTimeout: 10 * time.Second,
		},
	}

	return &Producer{
		writer: writer,
	}
}

// Publish sends a message to Kafka with exponential backoff retry.
// Retries up to publishMaxRetries times (100ms, 200ms, 400ms) before
// returning the error. Context cancellation is respected between retries.
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	msg := kafka.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	}

	var err error
	backoff := publishInitialBackoff

	for attempt := 1; attempt <= publishMaxRetries; attempt++ {
		err = p.writer.WriteMessages(ctx, msg)
		if err == nil {
			return nil
		}

		if attempt < publishMaxRetries {
			log.Printf("Kafka publish attempt %d/%d failed: %v — retrying in %s",
				attempt, publishMaxRetries, err, backoff)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			backoff *= 2
		}
	}

	log.Printf("CRITICAL: Kafka publish FAILED after %d attempts: %v — context update may be lost", publishMaxRetries, err)
	return err
}

// Close closes the producer
func (p *Producer) Close() error {
	return p.writer.Close()
}
