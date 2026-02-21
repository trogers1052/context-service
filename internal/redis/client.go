package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps the Redis client for context storage
type Client struct {
	client     *redis.Client
	contextKey string
}

// NewClient creates a new Redis client
func NewClient(host string, port int, password string, db int, contextKey string) *Client {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     5,
	})

	return &Client{
		client:     client,
		contextKey: contextKey,
	}
}

// Connect tests the Redis connection
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	log.Println("Connected to Redis")
	return nil
}

// PublishContext stores the market context in Redis
func (c *Client) PublishContext(ctx context.Context, contextJSON []byte) error {
	// Store the context with a TTL (in case service stops updating)
	err := c.client.Set(ctx, c.contextKey, contextJSON, 5*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("failed to publish context to Redis: %w", err)
	}

	// Also publish to a channel for real-time subscribers
	err = c.client.Publish(ctx, c.contextKey+":updates", contextJSON).Err()
	if err != nil {
		log.Printf("Warning: failed to publish to Redis channel: %v", err)
		// Don't return error - the key-based storage is more important
	}

	return nil
}

// GetContext retrieves the current market context from Redis
func (c *Client) GetContext(ctx context.Context) ([]byte, error) {
	result, err := c.client.Get(ctx, c.contextKey).Bytes()
	if err == redis.Nil {
		return nil, nil // No context stored yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get context from Redis: %w", err)
	}
	return result, nil
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}
