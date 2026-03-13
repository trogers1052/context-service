package service

import (
	"context"
	"log"
	"sync"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/kafka"
	"github.com/trogers1052/context-service/internal/macro"
	"github.com/trogers1052/context-service/internal/metrics"
	"github.com/trogers1052/context-service/internal/redis"
	"github.com/trogers1052/context-service/internal/regime"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// IndicatorEvent represents an incoming indicator message from analytics-service.
//
// analytics-service publishes indicators nested under data.indicators with
// UPPERCASE keys (e.g. "SMA_200", "RSI_14").  We unmarshal into a generic
// map and extract the fields we need with case-insensitive lookup so the
// context-service is robust to casing changes.
type IndicatorEvent struct {
	EventType string        `json:"event_type"`
	Data      IndicatorData `json:"data"`
}

// IndicatorData mirrors the "data" object in the Kafka message.
type IndicatorData struct {
	Symbol     string             `json:"symbol"`
	Time       string             `json:"time"`
	Indicators map[string]float64 `json:"indicators"`
}

// parsedIndicators holds the typed indicator values extracted from the map.
type parsedIndicators struct {
	Close         float64
	Volume        int64
	SMA20         float64
	SMA50         float64
	SMA200        float64
	RSI14         float64
	MACD          float64
	MACDSignal    float64
	MACDHistogram float64
	ATR14         float64
	VolumeSMA20   float64
	Timestamp     time.Time
}

// extractIndicators pulls typed values from the raw indicators map.
// analytics-service uses UPPERCASE keys (SMA_200, RSI_14, MACD, etc.)
func extractIndicators(d *IndicatorData) parsedIndicators {
	m := d.Indicators
	p := parsedIndicators{
		Close:         m["close"],
		Volume:        int64(m["volume"]),
		SMA20:         m["SMA_20"],
		SMA50:         m["SMA_50"],
		SMA200:        m["SMA_200"],
		RSI14:         m["RSI_14"],
		MACD:          m["MACD"],
		MACDSignal:    m["MACD_SIGNAL"],
		MACDHistogram: m["MACD_HISTOGRAM"],
		ATR14:         m["ATR_14"],
		VolumeSMA20:   m["volume_sma_20"],
	}
	if d.Time != "" {
		if t, err := time.Parse(time.RFC3339Nano, d.Time); err == nil {
			p.Timestamp = t
		}
	}
	return p
}

// ContextService is the main service orchestrator
type ContextService struct {
	config   *config.Config
	consumer *kafka.Consumer
	producer *kafka.Producer
	redis    *redis.Client
	detector *regime.Detector

	// macroFetcher polls FRED for VIX and HY spreads (nil when not configured).
	macroFetcher *macro.Fetcher

	// Track which symbols we care about
	trackedSymbols map[string]bool

	// Last published context (for change detection)
	lastContext     *regime.MarketContext
	lastContextLock sync.RWMutex

	// Publish interval
	publishInterval time.Duration
	lastPublish     time.Time

	// Rate-limit gate (separate from lastPublish so the 5-min heartbeat still works)
	rateLimitMu sync.Mutex
	lastAttempt time.Time

	// Stored context from Start() for use in callbacks
	ctx context.Context

	// wg tracks goroutines so Stop() can wait for them to finish.
	wg sync.WaitGroup
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

	// Initialize macro fetcher when FRED_API_KEY is set.
	// The first refresh is best-effort — a failure here is non-fatal so the
	// service can still run on technical indicators alone while FRED is down.
	if s.config.FREDAPIKey != "" {
		s.macroFetcher = macro.NewFetcher(s.config.FREDAPIKey)
		fredStart := time.Now()
		if err := s.macroFetcher.Refresh(); err != nil {
			log.Printf("Warning: initial macro fetch failed: %v — will retry every 4h", err)
			metrics.FredFetchErrors.Inc()
		} else {
			metrics.FredFetchDuration.Observe(time.Since(fredStart).Seconds())
			signals := s.macroFetcher.Get()
			if signals.Available {
				metrics.VixLevel.Set(signals.VIX)
			}
		}
	} else {
		log.Println("FRED_API_KEY not set — macro signals (VIX, HY spreads) disabled")
	}

	log.Println("Context service initialized")
	return nil
}

// handleMessage processes incoming indicator messages
func (s *ContextService) handleMessage(key, value []byte) error {
	var event IndicatorEvent
	if err := json.Unmarshal(value, &event); err != nil {
		log.Printf("Failed to unmarshal indicator event: %v", err)
		metrics.ParseErrors.Inc()
		return nil // Don't return error to avoid blocking consumer
	}

	// Only process symbols we care about
	if !s.trackedSymbols[event.Data.Symbol] {
		return nil
	}

	metrics.KafkaConsumed.WithLabelValues(event.Data.Symbol).Inc()

	// Extract typed indicators from the nested map
	p := extractIndicators(&event.Data)

	// Warn about zero-value indicators that likely indicate missing/incomplete data
	if p.SMA200 == 0 {
		log.Printf("Warning: received SMA200=0 for %s — treating as missing data", event.Data.Symbol)
	}
	if p.RSI14 == 0 {
		log.Printf("Warning: received RSI=0 for %s — treating as missing data", event.Data.Symbol)
	}

	// Update detector with new indicators
	indicators := &regime.Indicators{
		Symbol:        event.Data.Symbol,
		Close:         p.Close,
		SMA20:         p.SMA20,
		SMA50:         p.SMA50,
		SMA200:        p.SMA200,
		RSI14:         p.RSI14,
		MACD:          p.MACD,
		MACDSignal:    p.MACDSignal,
		MACDHistogram: p.MACDHistogram,
		ATR14:         p.ATR14,
		Volume:        p.Volume,
		VolumeSMA20:   p.VolumeSMA20,
		Timestamp:     p.Timestamp,
	}
	s.detector.UpdateIndicators(indicators)

	// Per-indicator updates are high-frequency; don't log every one.
	// Regime changes and context publishes are logged in publishContext.

	// Maybe publish updated context
	s.maybePublishContext()

	return nil
}

// maybePublishContext publishes context if enough time has passed
func (s *ContextService) maybePublishContext() {
	// Use a dedicated mutex for the rate-limit gate so lastPublish (used for the
	// 5-minute heartbeat check in hasContextChanged) is only updated on actual publish.
	s.rateLimitMu.Lock()
	if time.Since(s.lastAttempt) < s.publishInterval {
		s.rateLimitMu.Unlock()
		return
	}
	s.lastAttempt = time.Now() // claim the slot atomically
	s.rateLimitMu.Unlock()

	// Need sufficient data
	if !s.detector.HasSufficientData() {
		return
	}

	// Get current context
	marketCtx := s.detector.GetMarketContext()

	// Enrich with macro signals when available (VIX, HY credit spreads).
	// This may adjust the regime — e.g. VIX > 35 overrides BULL → BEAR.
	if s.macroFetcher != nil {
		regime.ApplyMacroAdjustments(marketCtx, s.macroFetcher.Get())
	}

	// Check if context has meaningfully changed
	if !s.hasContextChanged(marketCtx) {
		return
	}

	// Log regime detection result at publish time (not per-indicator)
	s.lastContextLock.RLock()
	prevRegime := "none"
	if s.lastContext != nil {
		prevRegime = string(s.lastContext.Regime)
	}
	s.lastContextLock.RUnlock()

	if prevRegime != string(marketCtx.Regime) {
		log.Printf("Regime change detected: %s -> %s (confidence=%.2f)",
			prevRegime, marketCtx.Regime, marketCtx.RegimeConfidence)
		metrics.RegimeTransitions.WithLabelValues(prevRegime, string(marketCtx.Regime)).Inc()
		metrics.SetCurrentRegime(string(marketCtx.Regime))
	}

	// Publish to Redis and Kafka
	s.publishContext(marketCtx)
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
func (s *ContextService) publishContext(marketCtx *regime.MarketContext) {
	contextJSON, err := json.Marshal(marketCtx)
	if err != nil {
		log.Printf("Failed to marshal context: %v", err)
		return
	}

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Determine publish reason for metrics.
	reason := "change_detected"
	s.lastContextLock.RLock()
	if s.lastContext != nil && marketCtx.Regime == s.lastContext.Regime {
		reason = "heartbeat"
	}
	s.lastContextLock.RUnlock()

	// Publish to Redis
	redisStart := time.Now()
	if err := s.redis.PublishContext(ctx, contextJSON); err != nil {
		log.Printf("Failed to publish to Redis: %v", err)
		metrics.RedisWriteErrors.Inc()
	} else {
		metrics.RedisWriteDuration.Observe(time.Since(redisStart).Seconds())
		log.Printf("Published context to Redis: regime=%s confidence=%.2f",
			marketCtx.Regime, marketCtx.RegimeConfidence)
	}

	// Publish to Kafka
	if err := s.producer.Publish(ctx, []byte("market"), contextJSON); err != nil {
		log.Printf("Failed to publish to Kafka: %v", err)
		metrics.KafkaPublishErrors.Inc()
	} else {
		metrics.KafkaPublished.WithLabelValues(reason).Inc()
		log.Printf("Published context to Kafka: regime=%s confidence=%.2f",
			marketCtx.Regime, marketCtx.RegimeConfidence)
	}

	// Update sector strength gauges
	for sector, strength := range marketCtx.SectorStrength {
		metrics.SectorStrength.WithLabelValues(sector).Set(strength)
	}

	// Update last published context and timestamp
	s.lastContextLock.Lock()
	s.lastContext = marketCtx
	s.lastPublish = time.Now()
	s.lastContextLock.Unlock()
}

// Start begins the service. The caller should cancel ctx to initiate shutdown,
// then call Stop() to wait for clean teardown.
func (s *ContextService) Start(ctx context.Context) error {
	s.ctx = ctx
	log.Println("========================================")
	log.Println("Starting Context Service")
	log.Println("========================================")
	log.Printf("Consuming from: %s", s.config.InputTopic)
	log.Printf("Publishing to: %s (Kafka) + %s (Redis)", s.config.OutputTopic, s.config.ContextKey)
	log.Printf("Tracking regime symbols: %v", s.config.RegimeSymbols)
	log.Printf("Tracking sector symbols: %v", s.config.SectorSymbols)
	log.Println("========================================")

	// Start macro refresher goroutine (polls FRED every 4 hours).
	if s.macroFetcher != nil {
		s.wg.Add(1)
		go s.runMacroRefresher(ctx)
	}

	s.wg.Add(1)
	defer s.wg.Done()

	return s.consumer.Start(ctx)
}

// runMacroRefresher polls the FRED API every 4 hours to refresh VIX and HY
// spread data.  It exits when ctx is cancelled (i.e., on graceful shutdown).
func (s *ContextService) runMacroRefresher(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(4 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fredStart := time.Now()
			if err := s.macroFetcher.Refresh(); err != nil {
				log.Printf("Macro refresh failed: %v", err)
				metrics.FredFetchErrors.Inc()
			} else {
				metrics.FredFetchDuration.Observe(time.Since(fredStart).Seconds())
				signals := s.macroFetcher.Get()
				log.Printf("FRED macro refresh: VIX=%.2f (%s), HY=%.2f%% (%s)",
					signals.VIX, signals.VIXLevel, signals.HYSpread, signals.HYLevel)
				if signals.Available {
					metrics.VixLevel.Set(signals.VIX)
				}
			}
		}
	}
}

// Stop gracefully shuts down the service. It waits for the consumer goroutine
// to finish (up to 10 seconds) before closing connections, preventing races
// where a goroutine uses a closed resource.
func (s *ContextService) Stop() {
	log.Println("Stopping context service...")

	// Wait for Start() goroutine to exit so we don't close resources out
	// from under it. Use a timeout to avoid hanging forever.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Consumer goroutine finished cleanly")
	case <-time.After(10 * time.Second):
		log.Println("Warning: timed out waiting for consumer goroutine to finish")
	}

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
