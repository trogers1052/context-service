package kafka

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
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
	}

	return &Producer{
		writer: writer,
	}
}

// Publish sends a message to Kafka
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	err := p.writer.WriteMessages(ctx, kafka.Message{
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})

	if err != nil {
		log.Printf("Error publishing to Kafka: %v", err)
		return err
	}

	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	return p.writer.Close()
}
