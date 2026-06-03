package config

import (
	"github.com/trogers1052/trading-go-commons/env"
)

// Config holds all configuration for the context service
type Config struct {
	// Kafka configuration
	KafkaBrokers  []string
	InputTopic    string // stock.indicators
	OutputTopic   string // market.context
	ConsumerGroup string

	// Redis configuration
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int
	ContextKey    string // market:context

	// Regime detection symbols
	RegimeSymbols []string // SPY, QQQ
	SectorSymbols []string // XLK, XLF, XLE, etc.

	// Macro enrichment (optional)
	FREDAPIKey string // FRED API key for VIX + HY spread fetching; empty disables macro signals

	// Service configuration
	LogLevel string
}

// Load creates a Config from environment variables
func Load() *Config {
	return &Config{
		KafkaBrokers:  env.StringSliceRaw("KAFKA_BROKERS", []string{"localhost:9092"}, ","),
		InputTopic:    env.String("KAFKA_INPUT_TOPIC", "stock.indicators"),
		OutputTopic:   env.String("KAFKA_OUTPUT_TOPIC", "market.context"),
		ConsumerGroup: env.String("KAFKA_CONSUMER_GROUP", "context-service"),

		RedisHost:     env.String("REDIS_HOST", "localhost"),
		RedisPort:     env.Int("REDIS_PORT", 6379),
		RedisPassword: env.String("REDIS_PASSWORD", ""),
		RedisDB:       env.Int("REDIS_DB", 0),
		ContextKey:    env.String("REDIS_CONTEXT_KEY", "market:context"),

		RegimeSymbols: env.StringSliceRaw("REGIME_SYMBOLS", []string{"SPY", "QQQ"}, ","),
		SectorSymbols: env.StringSliceRaw("SECTOR_SYMBOLS", []string{
			"XLK", "XLF", "XLE", "XLV", "XLI", "XLY", "XLP", "XLU",
			"XLB", "GDX", "GLD", "XME", "URA", "SIL", "REMX",
		}, ","),

		FREDAPIKey: env.String("FRED_API_KEY", ""),

		LogLevel: env.String("LOG_LEVEL", "info"),
	}
}
