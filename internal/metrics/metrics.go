// Package metrics defines application-level Prometheus metrics for the
// context-service.  All metrics are registered via promauto so they are
// automatically available on the /metrics endpoint served by promhttp.Handler().
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Kafka consumption
var (
	// KafkaConsumed counts indicator messages consumed from Kafka, by symbol.
	KafkaConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "context_kafka_consumed_total",
		Help: "Indicator messages consumed from Kafka.",
	}, []string{"symbol"})

	// KafkaPublished counts context messages published to Kafka, by reason
	// (change_detected or heartbeat).
	KafkaPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "context_kafka_published_total",
		Help: "Context messages published to Kafka.",
	}, []string{"reason"})

	// KafkaPublishErrors counts Kafka publish failures.
	KafkaPublishErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "context_kafka_publish_errors_total",
		Help: "Kafka publish failures.",
	})

	// KafkaReconnects counts Kafka consumer reconnections.
	KafkaReconnects = promauto.NewCounter(prometheus.CounterOpts{
		Name: "context_kafka_reconnects_total",
		Help: "Kafka consumer reconnections.",
	})
)

// Regime tracking
var (
	// RegimeTransitions counts regime changes, labelled by previous and new regime.
	RegimeTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "context_regime_transitions_total",
		Help: "Regime transitions.",
	}, []string{"from_regime", "to_regime"})

	// CurrentRegime is a gauge set to 1 for the active regime (for dashboards).
	// Set the active regime to 1 and all others to 0 on each transition.
	CurrentRegime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "context_current_regime",
		Help: "Current market regime (1 = active).",
	}, []string{"regime_id"})
)

// Parse errors
var (
	// ParseErrors counts indicator message parse/unmarshal failures.
	ParseErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "context_parse_errors_total",
		Help: "Indicator message parse failures.",
	})
)

// Redis
var (
	// RedisWriteDuration observes Redis write latency in seconds.
	RedisWriteDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "context_redis_write_duration_seconds",
		Help:    "Redis write latency.",
		Buckets: prometheus.DefBuckets,
	})

	// RedisWriteErrors counts Redis write failures.
	RedisWriteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "context_redis_write_errors_total",
		Help: "Redis write failures.",
	})
)

// FRED API
var (
	// FredFetchDuration observes FRED API call latency in seconds.
	FredFetchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "context_fred_fetch_duration_seconds",
		Help:    "FRED API call latency.",
		Buckets: prometheus.DefBuckets,
	})

	// FredFetchErrors counts FRED API call failures.
	FredFetchErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "context_fred_fetch_errors_total",
		Help: "FRED API call failures.",
	})
)

// Macro gauges
var (
	// VixLevel tracks the current VIX value from FRED.
	VixLevel = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "context_vix_level",
		Help: "Current VIX level from FRED.",
	})

	// SectorStrength tracks relative strength values per sector ETF.
	SectorStrength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "context_sector_strength",
		Help: "Sector relative strength vs SPY.",
	}, []string{"sector"})
)

// AllRegimes enumerates the known regime IDs so callers can reset the gauge.
var AllRegimes = []string{"BULL", "BEAR", "SIDEWAYS", "UNKNOWN"}

// SetCurrentRegime sets the given regime to 1 and all others to 0.
func SetCurrentRegime(active string) {
	for _, r := range AllRegimes {
		if r == active {
			CurrentRegime.WithLabelValues(r).Set(1)
		} else {
			CurrentRegime.WithLabelValues(r).Set(0)
		}
	}
}
