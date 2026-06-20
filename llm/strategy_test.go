package llm_test

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// TestSelectionStrategy_Random_DistributesFirstPick asserts that over 100 iterations
// with varying seeds, ≥2 distinct models are picked first, and each model appears
// first at least once.
func TestSelectionStrategy_Random_DistributesFirstPick(t *testing.T) {
	s1 := httptest.NewServer(okChatHandler(t, "m1"))
	defer s1.Close()
	s2 := httptest.NewServer(okChatHandler(t, "m2"))
	defer s2.Close()
	s3 := httptest.NewServer(okChatHandler(t, "m3"))
	defer s3.Close()

	firstPicks := make(map[string]int)

	for i := range 100 {
		var firstModel string
		obs := func(ep llm.Endpoint, err error) {
			if err == nil && firstModel == "" {
				firstModel = ep.Model
			}
		}
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: s1.URL, Key: "k", Model: "m1"},
				{URL: s2.URL, Key: "k", Model: "m2"},
				{URL: s3.URL, Key: "k", Model: "m3"},
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionRandom),
			llm.WithRander(rand.New(rand.NewSource(int64(i)))),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
		if firstModel != "" {
			firstPicks[firstModel]++
		}
	}

	if len(firstPicks) < 2 {
		t.Errorf("random strategy must distribute first-picks: got %v distinct first models over 100 runs", firstPicks)
	}
	for _, m := range []string{"m1", "m2", "m3"} {
		if firstPicks[m] == 0 {
			t.Errorf("model %q never picked first over 100 random runs (dist=%v)", m, firstPicks)
		}
	}
}

// TestSelectionStrategy_Priority_PreservesOrder: with SelectionPriority the first
// pick is always the primary (first endpoint), regardless of number of runs.
func TestSelectionStrategy_Priority_PreservesOrder(t *testing.T) {
	s1 := httptest.NewServer(okChatHandler(t, "primary"))
	defer s1.Close()
	s2 := httptest.NewServer(okChatHandler(t, "fallback"))
	defer s2.Close()

	for i := range 20 {
		var firstModel string
		obs := func(ep llm.Endpoint, _ error) {
			if firstModel == "" {
				firstModel = ep.Model
			}
		}
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: s1.URL, Key: "k", Model: "primary"},
				{URL: s2.URL, Key: "k", Model: "fallback"},
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionPriority),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
		if firstModel != "primary" {
			t.Errorf("iter %d: priority strategy: first pick = %q, want primary", i, firstModel)
		}
	}
}

// TestSelectionStrategy_Random_FallbackStillWorks: when the first shuffled model fails,
// the chain advances to the next model and overall call succeeds.
func TestSelectionStrategy_Random_FallbackStillWorks(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()
	okSrv := httptest.NewServer(okChatHandler(t, "from-fallback"))
	defer okSrv.Close()

	// With two endpoints (one always-fail, one always-ok), regardless of shuffle
	// order the call must succeed — chain advances past the failing endpoint.
	for seed := range int64(5) {
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: failSrv.URL, Key: "k", Model: "bad"},
				{URL: okSrv.URL, Key: "k", Model: "good"},
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionRandom),
			llm.WithRander(rand.New(rand.NewSource(seed))),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("seed %d: expected chain to advance past failing endpoint, got: %v", seed, err)
		}
	}
}

// TestSelectionStrategy_Random_CooledModelNeverSelected: a cooled model must
// not appear in the shuffled eligible list after entering cooldown.
func TestSelectionStrategy_Random_CooledModelNeverSelected(t *testing.T) {
	quota429Body := `{"error":{"message":"quota","type":"rate_limit_exceeded"}}`
	quotaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(quota429Body))
	}))
	defer quotaSrv.Close()
	okSrv := httptest.NewServer(okChatHandler(t, "from-healthy"))
	defer okSrv.Close()

	endpoints := []llm.Endpoint{
		{URL: quotaSrv.URL, Key: "k", Model: "quota-model"},
		{URL: okSrv.URL, Key: "k", Model: "healthy-model"},
	}
	mkClient := func(obs llm.EndpointAttemptObserver) *llm.Client {
		opts := []llm.Option{
			llm.WithEndpoints(endpoints),
			llm.WithMaxRetries(1),
			llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
			llm.WithSelectionStrategy(llm.SelectionRandom),
			llm.WithRander(rand.New(rand.NewSource(0))),
		}
		if obs != nil {
			opts = append(opts, llm.WithEndpointAttemptObserver(obs))
		}
		return llm.NewClient("", "", "", opts...)
	}

	// Warm up: make quota-model enter cooldown (FailThreshold=1, so 1 hit suffices).
	// We run calls until quota-model has been hit once (it may be shuffled to second).
	warmupClient := mkClient(nil)
	for range 3 {
		_, _ = warmupClient.Complete(context.Background(), "", "test")
	}

	// Now use a fresh client (shares no cooldown state) — warm it up independently.
	var attempted []string
	obs := func(ep llm.Endpoint, _ error) {
		attempted = append(attempted, ep.Model)
	}
	obsClient := mkClient(obs)

	// First call: quota-model may be tried (not yet cooled on this client).
	_, _ = obsClient.Complete(context.Background(), "", "test")
	attempted = attempted[:0] // reset

	// Second call: quota-model hit once → cooled. It must not appear.
	_, err := obsClient.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("post-cooldown call error: %v", err)
	}
	for _, m := range attempted {
		if m == "quota-model" {
			t.Errorf("cooled quota-model was attempted despite being in cooldown; attempts=%v", attempted)
		}
	}
}

// TestSelectionStrategy_InvalidEnvValue: parseSelectionStrategy falls back to
// SelectionPriority (and logs a warning) on an unknown value.
func TestSelectionStrategy_InvalidEnvValue(t *testing.T) {
	got := llm.ParseSelectionStrategy("bogus-value")
	if got != llm.SelectionPriority {
		t.Errorf("ParseSelectionStrategy(%q) = %v, want SelectionPriority (%v)", "bogus-value", got, llm.SelectionPriority)
	}
}

// TestSelectionStrategy_KnownValues: parseSelectionStrategy maps all defined strings.
func TestSelectionStrategy_KnownValues(t *testing.T) {
	cases := []struct {
		input string
		want  llm.SelectionStrategy
	}{
		{"priority", llm.SelectionPriority},
		{"", llm.SelectionPriority},
		{"random", llm.SelectionRandom},
	}
	for _, tc := range cases {
		got := llm.ParseSelectionStrategy(tc.input)
		if got != tc.want {
			t.Errorf("ParseSelectionStrategy(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
