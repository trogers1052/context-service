package sentiment_test

import (
	"testing"

	"github.com/trogers1052/context-service/internal/sentiment"
)

func TestStubFetcher_ReturnsUnavailable(t *testing.T) {
	s, err := sentiment.NewStubFetcher().Fetch()
	if err != nil {
		t.Fatalf("stub Fetch should not error, got %v", err)
	}
	if s.Available {
		t.Error("stub sentiment must report Available=false until a real source is wired")
	}
}

func TestClassifyFearGreed_Bands(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, sentiment.LabelExtremeFear},
		{25, sentiment.LabelExtremeFear},
		{26, sentiment.LabelFear},
		{45, sentiment.LabelFear},
		{46, sentiment.LabelNeutral},
		{54, sentiment.LabelNeutral},
		{55, sentiment.LabelGreed},
		{74, sentiment.LabelGreed},
		{75, sentiment.LabelExtremeGreed},
		{100, sentiment.LabelExtremeGreed},
	}
	for _, c := range cases {
		if got := sentiment.ClassifyFearGreed(c.v); got != c.want {
			t.Errorf("ClassifyFearGreed(%.0f): got %s, want %s", c.v, got, c.want)
		}
	}
}

// StubFetcher must satisfy the Fetcher interface so a real fetcher can replace it.
var _ sentiment.Fetcher = (*sentiment.StubFetcher)(nil)
