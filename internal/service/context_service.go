package service

import (
	"context"
	"log"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/kafka"
	"github.com/trogers1052/context-service/internal/redis"
	"github.com/trogers1052/context-service/internal/regime"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// IndicatorEvent represents an incoming indicator message from analytics-service
type IndicatorEvent struct {
	EventType string                 `json:"event_type"`
	Data      IndicatorData          `json:"data"`
}

// IndicatorData contains the indicator values
type IndicatorData struct {
	Symbol        string    `json:"symbol"`
	Timestamp     time.Time `json:"timestamp"`
	Close         float64   `json:"close"`
	Volume        int64     `json:"volume"`
	SMA20         float64   `json:"sma_20"`
	SMA50         float64   `json:"sma_50"`
	SMA200        float64   `json:"sma_200"`
	RSI14         float64   `json:"rsi_14"`
	MACD          float64   `json:"macd"`
	MACDSignal    float64   `json:"macd_signal"`
	MACDHistogram float64   `json:"macd_histogram"`
	ATR14         float64   `json:"atr_14"`
	VolumeSMA20   float64   `json:"volume_sma_20"`
}

// ContextService is the main service orchestrator
type ContextService struct {
	config   *config.Config
	consumer *kafka.Consumer
	producer *kafka.Producer
	redis    *redis.Client
	detector *regime.Detector

	// Track which symbols we care about
	trackedSymbols map[string]bool

	// Last published context (for change detection)
	lastContext     *regime.MarketContext
	lastContextLock sync.RWMutex

	// Publish interval
	publishInterval time.Duration
	lastPublish     time.Time
}

// NewContextService creates a new context service
func NewContextService(cfg *config.Config) *ContextService {
	// Build set of tracked symbols
	tracked := make(map[string]bool)
	for _, s := range cfg.RegimeSymbols {
		tracked[s] = true
	}
	for _, s := range cfg.SectorSymbols {
		tracked[s] = true
	}

	return &ContextService{
		config:          cfg,
		detector:        regime.NewDetector(cfg.RegimeSymbols, cfg.SectorSymbols),
		trackedSymbols:  tracked,
		publishInterval: 30 * time.Second, // Publish at most every 30 seconds
	}
}

// Initialize sets up all connections
func (s *ContextService) Initialize(ctx context.Context) error {
	// Initialize Kafka consumer
	s.consumer = kafka.NewConsumer(
		s.config.KafkaBrokers,
		s.config.InputTopic,
		s.config.ConsumerGroup,
		s.handleMessage,
	)

	// Initialize Kafka producer
	s.producer = kafka.NewProducer(
		s.config.KafkaBrokers,
		s.config.OutputTopic,
	)

	// Initialize Redis client
	s.redis = redis.NewClient(
		s.config.RedisHost,
		s.config.RedisPort,
		s.config.RedisPassword,
		s.config.RedisDB,
		s.config.ContextKey,
	)

	if err := s.redis.Connect(ctx); err != nil {
		return err
	}

	log.Println("Context service initialized")
	return nil
}

// handleMessage processes incoming indicator messages
func (s *ContextService) handleMessage(key, value []byte) error {
	var event IndicatorEvent
	if err := json.Unmarshal(value, &event); err != nil {
		log.Printf("Failed to unmarshal indicator event: %v", err)
		return nil // Don't return error to avoid blocking consumer
	}

	// Only process symbols we care about
	if !s.trackedSymbols[event.Data.Symbol] {
		return nil
	}

	// Update detector with new indicators
	indicators := &regime.Indicators{
		Symbol:        event.Data.Symbol,
		Close:         event.Data.Close,
		SMA20:         event.Data.SMA20,
		SMA50:         event.Data.SMA50,
		SMA200:        event.Data.SMA200,
		RSI14:         event.Data.RSI14,
		MACD:          event.Data.MACD,
		MACDSignal:    event.Data.MACDSignal,
		MACDHistogram: event.Data.MACDHistogram,
		ATR14:         event.Data.ATR14,
		Volume:        event.Data.Volume,
		VolumeSMA20:   event.Data.VolumeSMA20,
		Timestamp:     event.Data.Timestamp,
	}
	s.detector.UpdateIndicators(indicators)

	log.Printf("Updated indicators for %s (RSI: %.1f, Close: %.2f, SMA200: %.2f)",
		event.Data.Symbol, event.Data.RSI14, event.Data.Close, event.Data.SMA200)

	// Maybe publish updated context
	s.maybePublishContext()

	return nil
}

// maybePublishContext publishes context if enough time has passed
func (s *ContextService) maybePublishContext() {
	// Rate limit publishing
	if time.Since(s.lastPublish) < s.publishInterval {
		return
	}

	// Need sufficient data
	if !s.detector.HasSufficientData() {
		return
	}

	// Get current context
	ctx := s.detector.GetMarketContext()

	// Check if context has meaningfully changed
	if !s.hasContextChanged(ctx) {
		return
	}

	// Publish to Redis and Kafka
	s.publishContext(ctx)
}

// hasContextChanged checks if the context has meaningfully changed
func (s *ContextService) hasContextChanged(ctx *regime.MarketContext) bool {
	s.lastContextLock.RLock()
	defer s.lastContextLock.RUnlock()

	if s.lastContext == nil {
		return true
	}

	// Check for regime change
	if ctx.Regime != s.lastContext.Regime {
		return true
	}

	// Check for significant confidence change
	if abs(ctx.RegimeConfidence-s.lastContext.RegimeConfidence) > 0.1 {
		return true
	}

	// Publish periodically even without change
	if time.Since(s.lastPublish) > 5*time.Minute {
		return true
	}

	return false
}

// publishContext publishes the market context to Redis and Kafka
func (s *ContextService) publishContext(ctx *regime.MarketContext) {
	contextJSON, err := json.Marshal(ctx)
	if err != nil {
		log.Printf("Failed to marshal context: %v", err)
		return
	}

	// Publish to Redis
	if err := s.redis.PublishContext(context.Background(), contextJSON); err != nil {
		log.Printf("Failed to publish to Redis: %v", err)
	} else {
		log.Printf("Published context to Redis: regime=%s confidence=%.2f",
			ctx.Regime, ctx.RegimeConfidence)
	}

	// Publish to Kafka
	if err := s.producer.Publish(context.Background(), []byte("market"), contextJSON); err != nil {
		log.Printf("Failed to publish to Kafka: %v", err)
	}

	// Update last published
	s.lastContextLock.Lock()
	s.lastContext = ctx
	s.lastPublish = time.Now()
	s.lastContextLock.Unlock()
}

// Start begins the service
func (s *ContextService) Start(ctx context.Context) error {
	log.Println("========================================")
	log.Println("Starting Context Service")
	log.Println("========================================")
	log.Printf("Consuming from: %s", s.config.InputTopic)
	log.Printf("Publishing to: %s (Kafka) + %s (Redis)", s.config.OutputTopic, s.config.ContextKey)
	log.Printf("Tracking regime symbols: %v", s.config.RegimeSymbols)
	log.Printf("Tracking sector symbols: %v", s.config.SectorSymbols)
	log.Println("========================================")

	return s.consumer.Start(ctx)
}

// Stop gracefully shuts down the service
func (s *ContextService) Stop() {
	log.Println("Stopping context service...")

	if s.consumer != nil {
		s.consumer.Close()
	}
	if s.producer != nil {
		s.producer.Close()
	}
	if s.redis != nil {
		s.redis.Close()
	}

	log.Println("Context service stopped")
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
