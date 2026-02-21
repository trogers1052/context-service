package kafka

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

// MessageHandler is called for each consumed message
type MessageHandler func(key, value []byte) error

// Consumer consumes messages from a Kafka topic
type Consumer struct {
	reader  *kafka.Reader
	handler MessageHandler
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(brokers []string, topic, groupID string, handler MessageHandler) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		MaxWait:        500 * time.Millisecond,
		StartOffset:    kafka.LastOffset,
		CommitInterval: time.Second,
	})

	return &Consumer{
		reader:  reader,
		handler: handler,
	}
}

// Start begins consuming messages
func (c *Consumer) Start(ctx context.Context) error {
	log.Printf("Starting Kafka consumer for topic: %s", c.reader.Config().Topic)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("Error reading message: %v", err)
				continue
			}

			if err := c.handler(msg.Key, msg.Value); err != nil {
				log.Printf("Error handling message: %v", err)
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	return c.reader.Close()
}
