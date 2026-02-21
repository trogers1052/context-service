package redis

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	maxRetries     = 3
	baseRetryDelay = 500 * time.Millisecond
	maxRetryDelay  = 5 * time.Second
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

// isConnectionError checks if an error is a Redis connection-level error
// that warrants a retry (as opposed to a logic error like WRONGTYPE).
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	// Network-level errors (dial timeout, connection refused, broken pipe, etc.)
	if _, ok := err.(net.Error); ok {
		return true
	}
	// go-redis surfaces connection pool exhaustion and closed connections as
	// specific error strings; check for common patterns.
	s := err.Error()
	for _, sub := range []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"i/o timeout",
		"EOF",
		"use of closed network connection",
	} {
		if len(s) >= len(sub) && containsSubstring(s, sub) {
			return true
		}
	}
	return false
}

// containsSubstring is a simple contains check to avoid importing strings.
func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// retryOnConnectionError runs fn up to maxRetries times when fn returns a
// connection error. It uses exponential backoff capped at maxRetryDelay and
// respects context cancellation.
func (c *Client) retryOnConnectionError(ctx context.Context, operation string, fn func() error) error {
	var err error
	delay := baseRetryDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}

		if !isConnectionError(err) {
			// Logic error — retrying won't help.
			return err
		}

		if attempt == maxRetries {
			break
		}

		log.Printf("Redis %s failed (attempt %d/%d): %v — retrying in %v",
			operation, attempt+1, maxRetries+1, err, delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}

		// Exponential backoff
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}

		// Attempt a ping to verify the connection is back before retrying the
		// real operation. If the ping fails we still continue to the next
		// attempt — the pool may recover on its own.
		if pingErr := c.client.Ping(ctx).Err(); pingErr != nil {
			log.Printf("Redis reconnect ping failed: %v", pingErr)
		} else {
			log.Printf("Redis reconnected successfully")
		}
	}

	return fmt.Errorf("redis %s failed after %d retries: %w", operation, maxRetries+1, err)
}

// PublishContext stores the market context in Redis
func (c *Client) PublishContext(ctx context.Context, contextJSON []byte) error {
	// Store the context with a TTL (in case service stops updating)
	err := c.retryOnConnectionError(ctx, "SET", func() error {
		return c.client.Set(ctx, c.contextKey, contextJSON, 5*time.Minute).Err()
	})
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
	err := c.retryOnConnectionError(ctx, "GET", func() error {
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
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get context from Redis: %w", err)
	}
	return result, nil
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.client.Close()
}
