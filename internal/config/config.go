package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the context service
type Config struct {
	// Kafka configuration
	KafkaBrokers       []string
	InputTopic         string // stock.indicators
	OutputTopic        string // market.context
	ConsumerGroup      string

	// Redis configuration
	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int
	ContextKey    string // market:context

	// Regime detection symbols
	RegimeSymbols  []string // SPY, QQQ
	SectorSymbols  []string // XLK, XLF, XLE, etc.

	// Service configuration
	LogLevel string
}

// Load creates a Config from environment variables
func Load() *Config {
	return &Config{
		KafkaBrokers:  getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
		InputTopic:    getEnv("KAFKA_INPUT_TOPIC", "stock.indicators"),
		OutputTopic:   getEnv("KAFKA_OUTPUT_TOPIC", "market.context"),
		ConsumerGroup: getEnv("KAFKA_CONSUMER_GROUP", "context-service"),

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnvInt("REDIS_PORT", 6379),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		ContextKey:    getEnv("REDIS_CONTEXT_KEY", "market:context"),

		RegimeSymbols: getEnvSlice("REGIME_SYMBOLS", []string{"SPY", "QQQ"}),
		SectorSymbols: getEnvSlice("SECTOR_SYMBOLS", []string{
			"XLK", "XLF", "XLE", "XLV", "XLI", "XLY", "XLP", "XLU",
		}),

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
