# Context Service

Market regime detection and sector strength analysis for the trading platform.

## Overview

The Context Service provides real-time market context to enhance trading signals:

- **Regime Detection**: Analyzes SPY/QQQ indicators to classify market as BULL/BEAR/SIDEWAYS
- **Sector Strength**: Tracks sector ETFs relative to SPY to identify leaders/laggards
- **Macro Risk Signals**: Fetches VIX (equity fear gauge) and the ICE BofA US High Yield
  option-adjusted credit spread from the FRED API to classify volatility and credit-stress
  regimes (LOW/NORMAL/ELEVATED/CRISIS and TIGHT/NORMAL/WIDE/CRISIS), refreshed every 4 hours
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
| FRED_API_KEY | _(empty)_ | FRED API key for VIX + HY credit-spread fetching. When empty, macro signals are disabled and regime logic runs on technical indicators alone |
| LOG_LEVEL | info | Log verbosity |

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

## Macro Risk Signals (FRED)

When `FRED_API_KEY` is configured, the service fetches two daily series from the
[FRED API](https://fred.stlouisfed.org/) and uses them to enrich the published context:

| Series | FRED ID | Meaning | Levels |
|--------|---------|---------|--------|
| VIX | `VIXCLS` | CBOE Volatility Index (equity fear gauge) | LOW (<15), NORMAL, ELEVATED (>=25), CRISIS (>=35) |
| HY credit spread | `BAMLH0A0HYM2` | ICE BofA US High Yield option-adjusted spread (credit stress) | TIGHT (<3.5), NORMAL, WIDE (>=5.0), CRISIS (>=7.0) |

The macro fetcher refreshes every 4 hours. If FRED is unreachable, the cached value is
retained and regime logic treats missing macro signals as a no-op (permissive). When
`FRED_API_KEY` is unset, macro enrichment is skipped entirely.

## Future Enhancements

1. **News Sentiment**: Integrate Polygon.io news API for real-time sentiment
2. **Earnings Calendar**: Flag symbols with upcoming earnings
3. **Correlation Analysis**: Track sector correlations for diversification

---

## Built with Claude Code

A large portion of this project — implementation, tests, and documentation — was written in pair-programming sessions with [Claude Code](https://claude.com/claude-code), Anthropic's agentic command-line tool.
