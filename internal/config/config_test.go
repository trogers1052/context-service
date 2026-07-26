package config

import (
	"os"
	"testing"

	"github.com/trogers1052/trading-go-commons/env"
)

// ---- Load defaults ----------------------------------------------------------

func TestLoad_DefaultKafkaBrokers(t *testing.T) {
	cfg := Load()
	if len(cfg.KafkaBrokers) != 1 || cfg.KafkaBrokers[0] != "localhost:9092" {
		t.Errorf("KafkaBrokers: got %v, want [localhost:9092]", cfg.KafkaBrokers)
	}
}

func TestLoad_DefaultInputTopic(t *testing.T) {
	cfg := Load()
	if cfg.InputTopic != "stock.indicators" {
		t.Errorf("InputTopic: got %q, want %q", cfg.InputTopic, "stock.indicators")
	}
}

func TestLoad_DefaultOutputTopic(t *testing.T) {
	cfg := Load()
	if cfg.OutputTopic != "market.context" {
		t.Errorf("OutputTopic: got %q, want %q", cfg.OutputTopic, "market.context")
	}
}

func TestLoad_DefaultConsumerGroup(t *testing.T) {
	cfg := Load()
	if cfg.ConsumerGroup != "context-service" {
		t.Errorf("ConsumerGroup: got %q, want %q", cfg.ConsumerGroup, "context-service")
	}
}

func TestLoad_DefaultRedisHost(t *testing.T) {
	cfg := Load()
	if cfg.RedisHost != "localhost" {
		t.Errorf("RedisHost: got %q, want %q", cfg.RedisHost, "localhost")
	}
}

func TestLoad_DefaultRedisPort(t *testing.T) {
	cfg := Load()
	if cfg.RedisPort != 6379 {
		t.Errorf("RedisPort: got %d, want 6379", cfg.RedisPort)
	}
}

func TestLoad_DefaultRedisPassword(t *testing.T) {
	cfg := Load()
	if cfg.RedisPassword != "" {
		t.Errorf("RedisPassword: got %q, want empty", cfg.RedisPassword)
	}
}

func TestLoad_DefaultRedisDB(t *testing.T) {
	cfg := Load()
	if cfg.RedisDB != 0 {
		t.Errorf("RedisDB: got %d, want 0", cfg.RedisDB)
	}
}

func TestLoad_DefaultContextKey(t *testing.T) {
	cfg := Load()
	if cfg.ContextKey != "market:context" {
		t.Errorf("ContextKey: got %q, want %q", cfg.ContextKey, "market:context")
	}
}

func TestLoad_DefaultRegimeSymbols(t *testing.T) {
	cfg := Load()
	if len(cfg.RegimeSymbols) != 2 || cfg.RegimeSymbols[0] != "SPY" || cfg.RegimeSymbols[1] != "QQQ" {
		t.Errorf("RegimeSymbols: got %v, want [SPY QQQ]", cfg.RegimeSymbols)
	}
}

func TestLoad_DefaultSectorSymbols(t *testing.T) {
	cfg := Load()
	if len(cfg.SectorSymbols) != 15 {
		t.Errorf("SectorSymbols: got %d symbols, want 15", len(cfg.SectorSymbols))
	}
	expected := []string{
		"XLK", "XLF", "XLE", "XLV", "XLI", "XLY", "XLP", "XLU",
		"XLB", "GDX", "GLD", "XME", "URA", "SIL", "REMX",
	}
	for i, want := range expected {
		if i >= len(cfg.SectorSymbols) {
			t.Errorf("SectorSymbols[%d]: missing, want %q", i, want)
			continue
		}
		if cfg.SectorSymbols[i] != want {
			t.Errorf("SectorSymbols[%d]: got %q, want %q", i, cfg.SectorSymbols[i], want)
		}
	}
}

func TestLoad_DefaultFREDAPIKey_Empty(t *testing.T) {
	cfg := Load()
	if cfg.FREDAPIKey != "" {
		t.Errorf("FREDAPIKey: got %q, want empty", cfg.FREDAPIKey)
	}
}

func TestLoad_DefaultLogLevel(t *testing.T) {
	cfg := Load()
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "info")
	}
}

// ---- Environment variable overrides -----------------------------------------

func TestLoad_EnvOverride_KafkaBrokers(t *testing.T) {
	os.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
	defer os.Unsetenv("KAFKA_BROKERS")

	cfg := Load()
	if len(cfg.KafkaBrokers) != 2 {
		t.Fatalf("KafkaBrokers: got %d, want 2", len(cfg.KafkaBrokers))
	}
	if cfg.KafkaBrokers[0] != "broker1:9092" || cfg.KafkaBrokers[1] != "broker2:9092" {
		t.Errorf("KafkaBrokers: got %v", cfg.KafkaBrokers)
	}
}

func TestLoad_EnvOverride_InputTopic(t *testing.T) {
	os.Setenv("KAFKA_INPUT_TOPIC", "custom.input")
	defer os.Unsetenv("KAFKA_INPUT_TOPIC")

	cfg := Load()
	if cfg.InputTopic != "custom.input" {
		t.Errorf("InputTopic: got %q, want %q", cfg.InputTopic, "custom.input")
	}
}

func TestLoad_EnvOverride_OutputTopic(t *testing.T) {
	os.Setenv("KAFKA_OUTPUT_TOPIC", "custom.output")
	defer os.Unsetenv("KAFKA_OUTPUT_TOPIC")

	cfg := Load()
	if cfg.OutputTopic != "custom.output" {
		t.Errorf("OutputTopic: got %q, want %q", cfg.OutputTopic, "custom.output")
	}
}

func TestLoad_EnvOverride_RedisHost(t *testing.T) {
	os.Setenv("REDIS_HOST", "redis.local")
	defer os.Unsetenv("REDIS_HOST")

	cfg := Load()
	if cfg.RedisHost != "redis.local" {
		t.Errorf("RedisHost: got %q, want %q", cfg.RedisHost, "redis.local")
	}
}

func TestLoad_EnvOverride_RedisPort(t *testing.T) {
	os.Setenv("REDIS_PORT", "6380")
	defer os.Unsetenv("REDIS_PORT")

	cfg := Load()
	if cfg.RedisPort != 6380 {
		t.Errorf("RedisPort: got %d, want 6380", cfg.RedisPort)
	}
}

func TestLoad_EnvOverride_RedisPort_InvalidFallsBackToDefault(t *testing.T) {
	os.Setenv("REDIS_PORT", "not-a-number")
	defer os.Unsetenv("REDIS_PORT")

	cfg := Load()
	if cfg.RedisPort != 6379 {
		t.Errorf("RedisPort: got %d, want 6379 (default on invalid)", cfg.RedisPort)
	}
}

func TestLoad_EnvOverride_RedisDB(t *testing.T) {
	os.Setenv("REDIS_DB", "3")
	defer os.Unsetenv("REDIS_DB")

	cfg := Load()
	if cfg.RedisDB != 3 {
		t.Errorf("RedisDB: got %d, want 3", cfg.RedisDB)
	}
}

func TestLoad_EnvOverride_ContextKey(t *testing.T) {
	os.Setenv("REDIS_CONTEXT_KEY", "custom:key")
	defer os.Unsetenv("REDIS_CONTEXT_KEY")

	cfg := Load()
	if cfg.ContextKey != "custom:key" {
		t.Errorf("ContextKey: got %q, want %q", cfg.ContextKey, "custom:key")
	}
}

func TestLoad_EnvOverride_RegimeSymbols(t *testing.T) {
	os.Setenv("REGIME_SYMBOLS", "SPY,DIA,IWM")
	defer os.Unsetenv("REGIME_SYMBOLS")

	cfg := Load()
	if len(cfg.RegimeSymbols) != 3 {
		t.Fatalf("RegimeSymbols: got %d, want 3", len(cfg.RegimeSymbols))
	}
	if cfg.RegimeSymbols[2] != "IWM" {
		t.Errorf("RegimeSymbols[2]: got %q, want IWM", cfg.RegimeSymbols[2])
	}
}

func TestLoad_EnvOverride_FREDAPIKey(t *testing.T) {
	os.Setenv("FRED_API_KEY", "test-key-123")
	defer os.Unsetenv("FRED_API_KEY")

	cfg := Load()
	if cfg.FREDAPIKey != "test-key-123" {
		t.Errorf("FREDAPIKey: got %q, want %q", cfg.FREDAPIKey, "test-key-123")
	}
}

// FRED_BASE_URL must default to empty, which the service reads as "use the
// live API". A replay sets it to a stub serving historical series so that
// replaying 2021 does not classify regimes from today's macro data.
func TestLoad_DefaultFREDBaseURL_Empty(t *testing.T) {
	os.Unsetenv("FRED_BASE_URL")

	if cfg := Load(); cfg.FREDBaseURL != "" {
		t.Errorf("FREDBaseURL: got %q, want empty (live API)", cfg.FREDBaseURL)
	}
}

func TestLoad_EnvOverride_FREDBaseURL(t *testing.T) {
	os.Setenv("FRED_BASE_URL", "http://fred-stub:8080/fred/series/observations")
	defer os.Unsetenv("FRED_BASE_URL")

	cfg := Load()
	if cfg.FREDBaseURL != "http://fred-stub:8080/fred/series/observations" {
		t.Errorf("FREDBaseURL: got %q, want the stub URL", cfg.FREDBaseURL)
	}
}

func TestLoad_EnvOverride_LogLevel(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")

	cfg := Load()
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel: got %q, want %q", cfg.LogLevel, "debug")
	}
}

// ---- env.StringSliceRaw (raw comma-split for broker/symbol lists) -----------

func TestStringSliceRaw_ReturnsDefault_WhenNotSet(t *testing.T) {
	os.Unsetenv("TEST_GETENVSLICE_MISSING")
	got := env.StringSliceRaw("TEST_GETENVSLICE_MISSING", []string{"a", "b"}, ",")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want [a b]", got)
	}
}

func TestStringSliceRaw_SingleValue(t *testing.T) {
	os.Setenv("TEST_GETENVSLICE_ONE", "x")
	defer os.Unsetenv("TEST_GETENVSLICE_ONE")

	got := env.StringSliceRaw("TEST_GETENVSLICE_ONE", []string{"default"}, ",")
	if len(got) != 1 || got[0] != "x" {
		t.Errorf("got %v, want [x]", got)
	}
}

func TestStringSliceRaw_MultipleValues(t *testing.T) {
	os.Setenv("TEST_GETENVSLICE_MULTI", "x,y,z")
	defer os.Unsetenv("TEST_GETENVSLICE_MULTI")

	got := env.StringSliceRaw("TEST_GETENVSLICE_MULTI", nil, ",")
	if len(got) != 3 || got[0] != "x" || got[1] != "y" || got[2] != "z" {
		t.Errorf("got %v, want [x y z]", got)
	}
}

func TestStringSliceRaw_EmptyString_ReturnsDefault(t *testing.T) {
	os.Setenv("TEST_GETENVSLICE_EMPTY", "")
	defer os.Unsetenv("TEST_GETENVSLICE_EMPTY")

	got := env.StringSliceRaw("TEST_GETENVSLICE_EMPTY", []string{"a"}, ",")
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("got %v, want [a] (empty string uses default)", got)
	}
}
