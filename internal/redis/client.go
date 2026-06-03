package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/trogers1052/trading-go-commons/redisx"
)

// maxAttempts is the total number of tries (initial + retries) for a Redis
// operation that fails with a connection-level error.
const maxAttempts = 4

// Client wraps the Redis client for context storage
type Client struct {
	client     *redis.Client
	contextKey string
}

// NewClient creates a new Redis client
func NewClient(host string, port int, password string, db int, contextKey string) *Client {
	client := redisx.NewClient(redisx.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
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
	err := redisx.RetryOnConnectionError(ctx, func() error {
		return c.client.Set(ctx, c.contextKey, contextJSON, 5*time.Minute).Err()
	}, maxAttempts)
	if err != nil {
		return fmt.Errorf("failed to publish context to Redis: %w", err)
	}

	// Also publish to a channel for real-time subscribers
	pubErr := c.client.Publish(ctx, c.contextKey+":updates", contextJSON).Err()
	if pubErr != nil {
		log.Printf("Warning: failed to publish to Redis channel: %v", pubErr)
		// Don't return error - the key-based storage is more important
	}

	return nil
}

// GetContext retrieves the current market context from Redis
func (c *Client) GetContext(ctx context.Context) ([]byte, error) {
	var result []byte
	err := redisx.RetryOnConnectionError(ctx, func() error {
		val, getErr := c.client.Get(ctx, c.contextKey).Bytes()
		if getErr == redis.Nil {
			result = nil
			return nil // Not a connection error — no retry
		}
		if getErr != nil {
			return getErr
		}
		result = val
		return nil
	}, maxAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get context from Redis: %w", err)
	}
	return result, nil
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}
