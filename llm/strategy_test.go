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
// with varying seeds, >=2 distinct models are picked first, and each model appears
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

// TestSelectionStrategy_Random_CooledModelNeverSelected verifies that a cooled
// model is excluded from the eligible list (Guard A: the eligible-filter that
// builds a non-cooled subset before shuffling) and from the iteration loop
// (Guard B: the per-ep cooling() check inside the for range loop) after entering
// cooldown.
//
// Uses ONE client -- cooldown state is shared with the observed path. Drives
// quota-model into cooldown via real Complete() calls (same code path as production).
//
// Mutation proof (both guards must individually cause RED):
//   - Delete Guard A (eligible-filter in executeInner, skipCooled branch):
//     quota-model re-enters the shuffle and will be attempted. RED.
//   - Delete Guard B (loop-level "if skipCooled && c.cooldown.cooling(ep.Model) { continue }"):
//     quota-model is iterated even when it was not filtered by Guard A. RED.
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

	var attempted []string
	obs := func(ep llm.Endpoint, _ error) {
		attempted = append(attempted, ep.Model)
	}

	// ONE client -- cooldown recorded here feeds the same modelCooldown instance
	// checked by Guard A and Guard B. WithRander injects a fixed seed so the
	// shuffle sequence is deterministic and the test is reproducible.
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: quotaSrv.URL, Key: "k", Model: "quota-model"},
			{URL: okSrv.URL, Key: "k", Model: "healthy-model"},
		}),
		llm.WithMaxRetries(1),
		// FailThreshold=1: a single 429 cools quota-model immediately.
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
		llm.WithSelectionStrategy(llm.SelectionRandom),
		llm.WithRander(rand.New(rand.NewSource(0))),
		llm.WithEndpointAttemptObserver(obs),
	)

	// Phase 1 -- warmup: make calls until quota-model is attempted at least once.
	// The 429 from quotaSrv drives recordFailure on THIS client's cooldown.
	const maxWarmup = 8
	hitQuotaInWarmup := false
	for warmup := range maxWarmup {
		attempted = attempted[:0]
		_, _ = c.Complete(context.Background(), "", "test")
		for _, m := range attempted {
			if m == "quota-model" {
				hitQuotaInWarmup = true
			}
		}
		if hitQuotaInWarmup {
			t.Logf("warmup: quota-model hit after %d call(s)", warmup+1)
			break
		}
	}
	if !hitQuotaInWarmup {
		t.Skip("quota-model never appeared in warmup phase across 8 shuffles; degenerate seed sequence -- skip to avoid false-FAIL")
	}

	// Phase 2 -- verify: quota-model is now cooled (FailThreshold=1, one 429 sufficient).
	// Run N calls and assert quota-model never appears in the attempted list.
	const checkCalls = 8
	attempted = attempted[:0]

	for i := range checkCalls {
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("post-cooldown call %d: unexpected error (healthy-model should serve): %v", i, err)
		}
	}

	for _, m := range attempted {
		if m == "quota-model" {
			t.Errorf("cooled quota-model was attempted in a post-cooldown call; guard missing or cooldown not recorded on this client; full attempts=%v", attempted)
			break
		}
	}

	// Sanity: healthy-model must have been tried to confirm the chain ran at all.
	healthySeen := false
	for _, m := range attempted {
		if m == "healthy-model" {
			healthySeen = true
		}
	}
	if !healthySeen {
		t.Errorf("healthy-model never attempted in %d post-cooldown calls -- chain broken; attempts=%v", checkCalls, attempted)
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

// TestSelectionStrategy_EnvWiring: NewClient reads LLM_SELECTION_STRATEGY from
// the environment automatically. Covers four behaviours:
//   - "random" -> SelectionRandom (distributes picks)
//   - invalid value -> SelectionPriority + warn (always picks primary first)
//   - empty/unset -> SelectionPriority silently
//   - explicit WithSelectionStrategy option overrides env
func TestSelectionStrategy_EnvWiring(t *testing.T) {
	s1 := httptest.NewServer(okChatHandler(t, "m1"))
	defer s1.Close()
	s2 := httptest.NewServer(okChatHandler(t, "m2"))
	defer s2.Close()
	s3 := httptest.NewServer(okChatHandler(t, "m3"))
	defer s3.Close()

	endpoints3 := []llm.Endpoint{
		{URL: s1.URL, Key: "k", Model: "m1"},
		{URL: s2.URL, Key: "k", Model: "m2"},
		{URL: s3.URL, Key: "k", Model: "m3"},
	}

	t.Run("random env distributes first picks", func(t *testing.T) {
		t.Setenv("LLM_SELECTION_STRATEGY", "random")
		firstPicks := make(map[string]int)
		for i := range 50 {
			var firstModel string
			obs := func(ep llm.Endpoint, err error) {
				if err == nil && firstModel == "" {
					firstModel = ep.Model
				}
			}
			// No explicit WithSelectionStrategy -- NewClient must read env.
			c := llm.NewClient("", "", "",
				llm.WithEndpoints(endpoints3),
				llm.WithMaxRetries(1),
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
			t.Errorf("LLM_SELECTION_STRATEGY=random must distribute first-picks: got %v", firstPicks)
		}
	})

	t.Run("invalid env falls back to priority", func(t *testing.T) {
		t.Setenv("LLM_SELECTION_STRATEGY", "invalid-strategy")
		for i := range 10 {
			var firstModel string
			obs := func(ep llm.Endpoint, _ error) {
				if firstModel == "" {
					firstModel = ep.Model
				}
			}
			c := llm.NewClient("", "", "",
				llm.WithEndpoints([]llm.Endpoint{
					{URL: s1.URL, Key: "k", Model: "m1"},
					{URL: s2.URL, Key: "k", Model: "m2"},
				}),
				llm.WithMaxRetries(1),
				llm.WithEndpointAttemptObserver(obs),
			)
			_, err := c.Complete(context.Background(), "", "test")
			if err != nil {
				t.Fatalf("iter %d: unexpected error: %v", i, err)
			}
			if firstModel != "m1" {
				t.Errorf("iter %d: invalid env must fall back to priority: first=%q, want m1", i, firstModel)
			}
		}
	})

	t.Run("explicit WithSelectionStrategy overrides env", func(t *testing.T) {
		// Env says random; explicit option says priority -- option must win.
		t.Setenv("LLM_SELECTION_STRATEGY", "random")
		for i := range 10 {
			var firstModel string
			obs := func(ep llm.Endpoint, _ error) {
				if firstModel == "" {
					firstModel = ep.Model
				}
			}
			c := llm.NewClient("", "", "",
				llm.WithEndpoints([]llm.Endpoint{
					{URL: s1.URL, Key: "k", Model: "m1"},
					{URL: s2.URL, Key: "k", Model: "m2"},
				}),
				llm.WithMaxRetries(1),
				llm.WithSelectionStrategy(llm.SelectionPriority), // explicit override
				llm.WithEndpointAttemptObserver(obs),
			)
			_, err := c.Complete(context.Background(), "", "test")
			if err != nil {
				t.Fatalf("iter %d: unexpected error: %v", i, err)
			}
			if firstModel != "m1" {
				t.Errorf("iter %d: explicit WithSelectionStrategy(Priority) must override LLM_SELECTION_STRATEGY=random: first=%q", i, firstModel)
			}
		}
	})

	t.Run("empty env uses priority silently", func(t *testing.T) {
		t.Setenv("LLM_SELECTION_STRATEGY", "")
		var firstModel string
		obs := func(ep llm.Endpoint, _ error) {
			if firstModel == "" {
				firstModel = ep.Model
			}
		}
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: s1.URL, Key: "k", Model: "m1"},
				{URL: s2.URL, Key: "k", Model: "m2"},
			}),
			llm.WithMaxRetries(1),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if firstModel != "m1" {
			t.Errorf("empty env must use priority silently: first=%q, want m1", firstModel)
		}
	})
}
