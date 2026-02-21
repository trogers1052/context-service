# Context Service

Market regime detection and sector strength analysis for the trading platform.

## Overview

The Context Service provides real-time market context to enhance trading signals:

- **Regime Detection**: Analyzes SPY/QQQ indicators to classify market as BULL/BEAR/SIDEWAYS
- **Sector Strength**: Tracks sector ETFs relative to SPY to identify leaders/laggards
- **Future**: News sentiment integration (Polygon.io news API)

## Architecture

```
stock.indicators (Kafka)
        │
        ▼
┌─────────────────────────────┐
│     Context Service (Go)    │
│                             │
│  • Consumes SPY/QQQ/sectors │
│  • Detects market regime    │
│  • Calculates sector strength│
│  • Publishes context        │
└─────────────────────────────┘
        │
        ├──▶ Redis: market:context
        │
        └──▶ Kafka: market.context
                │
                ▼
        Decision-Engine
        (adjusts confidence based on regime)
```

## Regime Detection Logic

A symbol is classified based on three conditions:

| Condition | Bullish When |
|-----------|-------------|
| Price vs SMA200 | Close > SMA200 |
| RSI | RSI_14 > 50 |
| MACD | MACD > MACD_Signal |

**Classification:**
- **BULL**: 3/3 bullish (or 2/3 with price > SMA200)
- **BEAR**: 0/3 bullish
- **SIDEWAYS**: Mixed signals

**Overall Market Regime:**
- SPY and QQQ both BULL → Market BULL
- SPY and QQQ both BEAR → Market BEAR
- Disagreement → SIDEWAYS

## Output Format

```json
{
  "regime": "BULL",
  "regime_confidence": 0.85,
  "spy_regime": {
    "symbol": "SPY",
    "regime": "BULL",
    "confidence": 0.9,
    "above_sma_200": true,
    "rsi_bullish": true,
    "macd_bullish": true,
    "trend_strength": 3.2
  },
  "qqq_regime": { ... },
  "sector_strength": {
    "XLK": 1.5,
    "XLF": -0.8,
    "XLE": -2.1
  },
  "sector_leaders": ["XLK", "XLY"],
  "sector_laggards": ["XLE", "XLU"],
  "timestamp": "2026-02-20T14:30:00Z"
}
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| KAFKA_BROKERS | localhost:9092 | Kafka broker addresses |
| KAFKA_INPUT_TOPIC | stock.indicators | Input topic |
| KAFKA_OUTPUT_TOPIC | market.context | Output topic |
| KAFKA_CONSUMER_GROUP | context-service | Consumer group |
| REDIS_HOST | localhost | Redis host |
| REDIS_PORT | 6379 | Redis port |
| REDIS_PASSWORD | | Redis password |
| REDIS_DB | 0 | Redis database |
| REDIS_CONTEXT_KEY | market:context | Redis key for context |
| REGIME_SYMBOLS | SPY,QQQ | Symbols for regime detection |
| SECTOR_SYMBOLS | XLK,XLF,XLE,... | Sector ETFs to track |

## Running Locally

```bash
# Set environment variables
export KAFKA_BROKERS=localhost:9092
export REDIS_HOST=localhost

# Run
go run ./cmd/context
```

## Docker

```bash
# Build
docker build -t context-service .

# Run
docker run -e KAFKA_BROKERS=redpanda:9092 -e REDIS_HOST=redis context-service
```

## Integration with Decision-Engine

The decision-engine reads `market:context` from Redis before evaluating rules:

```python
# In decision-engine
context = redis.get("market:context")
if context["regime"] == "BEAR":
    confidence *= 0.8  # Lower confidence in bear market
```

## Future Enhancements

1. **News Sentiment**: Integrate Polygon.io news API for real-time sentiment
2. **VIX Integration**: Add volatility regime detection
3. **Earnings Calendar**: Flag symbols with upcoming earnings
4. **Correlation Analysis**: Track sector correlations for diversification
