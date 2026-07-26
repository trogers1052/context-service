package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	commonskafka "github.com/trogers1052/trading-go-commons/kafka"
	testkit "github.com/trogers1052/trading-testkit"

	"github.com/trogers1052/context-service/internal/config"
	"github.com/trogers1052/context-service/internal/kafka"
	"github.com/trogers1052/context-service/internal/redis"
)

// TestIntegration_RealBroker_ConsumeProduceRoundTrip exercises the migrated,
// kafka-go-based kafka.Consumer + kafka.Producer wrappers against a REAL broker
// (Redpanda via testkit). It drives the full service path:
//
//	input topic -> service.Consumer -> handleMessage -> regime detector ->
//	service.Producer -> output topic
//
// and asserts that publishing indicator events for the tracked regime symbols
// causes the service to publish a market.context message to the output topic.
//
// testkit.NewRedpandaContainer auto-skips under `go test -short`, so this test
// runs only when Docker is available and -short is not set.
func TestIntegration_RealBroker_ConsumeProduceRoundTrip(t *testing.T) {
	rp := testkit.NewRedpandaContainer(t)

	const (
		inputTopic  = "stock.indicators"
		outputTopic = "market.context"
		group       = "context-service-itest"
	)
	rp.CreateTopic(t, inputTopic, 1)
	rp.CreateTopic(t, outputTopic, 1)

	// In-process Redis so publishContext's Redis write succeeds.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	defer mr.Close()
	redisPort, _ := strconv.Atoi(mr.Port())

	// Build a real service wired to the live broker + miniredis.
	cfg := &config.Config{
		KafkaBrokers:  []string{rp.Brokers},
		InputTopic:    inputTopic,
		OutputTopic:   outputTopic,
		ConsumerGroup: group,
		RegimeSymbols: []string{"SPY", "QQQ"},
		SectorSymbols: []string{"XLK"},
		ContextKey:    "market:context",
	}
	svc := NewContextService(cfg, nil)

	svc.consumer = kafka.NewConsumer(cfg.KafkaBrokers, cfg.InputTopic, cfg.ConsumerGroup, svc.handleMessage)
	svc.producer = kafka.NewProducer(cfg.KafkaBrokers, cfg.OutputTopic)
	svc.redis = redis.NewClient(mr.Host(), redisPort, "", 0, cfg.ContextKey)
	if connErr := svc.redis.Connect(context.Background()); connErr != nil {
		t.Fatalf("redis.Connect: %v", connErr)
	}
	// Publish immediately on first sufficient data (don't wait out the 30s gate).
	svc.publishInterval = 0

	// Verification consumer on the OUTPUT topic, reading the backlog so we catch
	// the service's published context regardless of join timing.
	gotContext := make(chan []byte, 4)
	verifyHandler := func(_ context.Context, msg *commonskafka.Message) error {
		gotContext <- msg.Value
		return nil
	}
	verifyCG, err := commonskafka.NewConsumerGroup(
		[]string{rp.Brokers}, "verify-output", []string{outputTopic}, verifyHandler,
		commonskafka.WithInitialOffset(commonskafka.OffsetOldest),
	)
	if err != nil {
		t.Fatalf("verify NewConsumerGroup: %v", err)
	}
	defer verifyCG.Close()
	verifyCtx, verifyCancel := context.WithCancel(context.Background())
	verifyDone := make(chan struct{})
	go func() { _ = verifyCG.Run(verifyCtx); close(verifyDone) }()
	defer func() {
		verifyCancel()
		select {
		case <-verifyDone:
		case <-time.After(15 * time.Second):
			t.Error("verify consumer did not shut down")
		}
	}()

	// Start the service (blocks in consumer.Start until ctx cancelled).
	svcCtx, svcCancel := context.WithCancel(context.Background())
	svcDone := make(chan error, 1)
	go func() { svcDone <- svc.Start(svcCtx) }()
	defer func() {
		svcCancel()
		svc.Stop()
		select {
		case <-svcDone:
		case <-time.After(15 * time.Second):
			t.Error("service Start did not return after shutdown")
		}
	}()

	// The service consumer uses OffsetNewest, so wait for it to join and settle
	// on the latest offset before producing input it must see.
	time.Sleep(8 * time.Second)

	// An independent producer to feed the INPUT topic with indicator events.
	// kafka.NewProducer dials lazily, so it cannot fail here.
	inProd := kafka.NewProducer([]string{rp.Brokers}, inputTopic)
	defer inProd.Close()

	indicator := func(symbol string) []byte {
		return []byte(`{"event_type":"indicators","data":{"symbol":"` + symbol + `",` +
			`"time":"2026-06-03T15:00:00Z","indicators":{` +
			`"close":450,"SMA_20":440,"SMA_200":420,"RSI_14":60,` +
			`"MACD":1.0,"MACD_SIGNAL":0.5}}}`)
	}

	// Feed both regime symbols so HasSufficientData() becomes true and the
	// detector produces a real regime. Repeat a few times to ride past any
	// transient join races.
	deadline := time.After(45 * time.Second)
	pubTicker := time.NewTicker(2 * time.Second)
	defer pubTicker.Stop()

	// Initial burst.
	for _, sym := range []string{"SPY", "QQQ"} {
		if perr := inProd.Publish(context.Background(), []byte(sym), indicator(sym)); perr != nil {
			t.Fatalf("publish input %s: %v", sym, perr)
		}
	}

	for {
		select {
		case v := <-gotContext:
			if len(v) == 0 {
				t.Fatal("received empty market.context message")
			}
			// Sanity-check it's the marshalled MarketContext (has a regime field).
			if !containsKey(v, "regime") {
				t.Fatalf("output message missing expected 'regime' field: %s", v)
			}
			return // success
		case <-pubTicker.C:
			for _, sym := range []string{"SPY", "QQQ"} {
				_ = inProd.Publish(context.Background(), []byte(sym), indicator(sym))
			}
		case <-deadline:
			t.Fatal("did not observe a published market.context message within deadline")
		}
	}
}

// containsKey reports whether the JSON blob contains the given key name. A
// lightweight check that avoids importing a JSON lib just to assert presence.
func containsKey(blob []byte, key string) bool {
	needle := `"` + key + `"`
	return indexOf(string(blob), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
