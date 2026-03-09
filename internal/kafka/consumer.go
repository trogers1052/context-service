package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/trogers1052/context-service/internal/metrics"
)

const (
	maxReconnectAttempts = 5
	baseReconnectDelay   = 2 * time.Second
	maxReconnectDelay    = 60 * time.Second
	// consecutiveErrorThreshold is the number of read errors in a row before
	// the consumer tears down the reader and reconnects.
	consecutiveErrorThreshold = 5
)

// MessageHandler is called for each consumed message
type MessageHandler func(key, value []byte) error

// Consumer consumes messages from a Kafka topic
type Consumer struct {
	reader  *kafka.Reader
	handler MessageHandler
	config  kafka.ReaderConfig
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(brokers []string, topic, groupID string, handler MessageHandler) *Consumer {
	cfg := kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.LastOffset,
		Dialer: &kafka.Dialer{
			Timeout:   10 * time.Second,
			DualStack: true,
		},
	}

	return &Consumer{
		reader:  kafka.NewReader(cfg),
		handler: handler,
		config:  cfg,
	}
}

// newReader creates a fresh kafka.Reader from the stored config.
func (c *Consumer) newReader() *kafka.Reader {
	return kafka.NewReader(c.config)
}

// Start begins consuming messages. If the reader encounters too many
// consecutive errors it will close the current reader, wait with
// exponential backoff, create a new reader, and resume consuming.
// After maxReconnectAttempts consecutive reconnection failures it
// returns an error. A single successful read resets the retry counter.
func (c *Consumer) Start(ctx context.Context) error {
	log.Printf("Starting Kafka consumer for topic: %s", c.config.Topic)

	reconnectAttempts := 0
	reconnectDelay := baseReconnectDelay

	for {
		hadSuccess, err := c.consumeLoop(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// If the loop successfully read at least one message before failing,
		// the connection was alive at some point — reset the retry state so
		// transient blips don't accumulate toward the hard limit.
		if hadSuccess {
			reconnectAttempts = 0
			reconnectDelay = baseReconnectDelay
		}

		// consumeLoop only returns when ctx is cancelled or after hitting
		// consecutiveErrorThreshold errors in a row.
		reconnectAttempts++
		if reconnectAttempts > maxReconnectAttempts {
			return fmt.Errorf("kafka consumer giving up after %d reconnect attempts: %w",
				maxReconnectAttempts, err)
		}

		log.Printf("Kafka consumer disconnected (attempt %d/%d): %v — reconnecting in %v",
			reconnectAttempts, maxReconnectAttempts, err, reconnectDelay)

		// Close the old reader (best-effort).
		if closeErr := c.reader.Close(); closeErr != nil {
			log.Printf("Warning: error closing old Kafka reader: %v", closeErr)
		}

		// Wait before reconnecting.
		select {
		case <-time.After(reconnectDelay):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff for the next attempt.
		reconnectDelay *= 2
		if reconnectDelay > maxReconnectDelay {
			reconnectDelay = maxReconnectDelay
		}

		// Create a fresh reader.
		c.reader = c.newReader()
		metrics.KafkaReconnects.Inc()
		log.Printf("Created new Kafka reader for topic: %s", c.config.Topic)
	}
}

// consumeLoop reads messages until the context is cancelled or
// consecutiveErrorThreshold read errors occur in a row. It returns
// whether at least one message was successfully read (hadSuccess)
// so the caller can decide whether to reset the reconnect counter.
func (c *Consumer) consumeLoop(ctx context.Context) (hadSuccess bool, err error) {
	consecutiveErrors := 0
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			return hadSuccess, ctx.Err()
		default:
			msg, readErr := c.reader.FetchMessage(ctx)
			if readErr != nil {
				if ctx.Err() != nil {
					return hadSuccess, ctx.Err()
				}
				consecutiveErrors++
				lastErr = readErr
				log.Printf("Error reading message (%d/%d): %v",
					consecutiveErrors, consecutiveErrorThreshold, readErr)

				if consecutiveErrors >= consecutiveErrorThreshold {
					return hadSuccess, lastErr
				}

				// Brief pause before retrying within the same reader.
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					return hadSuccess, ctx.Err()
				}
				continue
			}

			// Successful read — reset error counter and mark success.
			consecutiveErrors = 0
			hadSuccess = true

			if handleErr := c.handler(msg.Key, msg.Value); handleErr != nil {
				log.Printf("Error handling message: %v", handleErr)
				// Don't commit — message will be redelivered on restart
				continue
			}

			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				log.Printf("Error committing offset: %v", commitErr)
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	return c.reader.Close()
}
