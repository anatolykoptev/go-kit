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

// --- SelectionWeighted tests below ---

// TestWeightedSelection_ProportionalFirstPick asserts that over 1000 iterations,
// the first-pick frequency is proportional to weight. m1=4, m2=4, m3=1 means
// m1+m2 combined should dominate m3 by roughly 8:1.
func TestWeightedSelection_ProportionalFirstPick(t *testing.T) {
	s1 := httptest.NewServer(okChatHandler(t, "m1"))
	defer s1.Close()
	s2 := httptest.NewServer(okChatHandler(t, "m2"))
	defer s2.Close()
	s3 := httptest.NewServer(okChatHandler(t, "m3"))
	defer s3.Close()

	rng := rand.New(rand.NewSource(42))
	firstPicks := make(map[string]int)

	for range 1000 {
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
			llm.WithSelectionStrategy(llm.SelectionWeighted),
			llm.WithModelWeights(map[string]int{"m1": 4, "m2": 4, "m3": 1}),
			llm.WithRander(rng),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if firstModel != "" {
			firstPicks[firstModel]++
		}
	}

	combined := firstPicks["m1"] + firstPicks["m2"]
	m3picks := firstPicks["m3"]
	if combined == 0 || m3picks == 0 {
		t.Fatalf("proportional test broken: m1+m2=%d, m3=%d (dist=%v)", combined, m3picks, firstPicks)
	}
	// 8:1 theoretical; allow generous margin — at least 4:1 expected at N=1000.
	ratio := float64(combined) / float64(m3picks)
	if ratio < 4.0 {
		t.Errorf("expected m1+m2 >> m3 (ratio ~8:1), got %.2f:1 (dist=%v)", ratio, firstPicks)
	}
	// m3 must appear at least once (not excluded, weight > 0).
	if m3picks == 0 {
		t.Errorf("m3 (weight=1) never picked first; should appear occasionally: dist=%v", firstPicks)
	}
}

// TestWeightedSelection_WeightZeroNeverAttempted asserts that a model with weight=0
// never appears in ANY attempt position, even when all positive-weight models fail
// (i.e. the loop falls through). This is a LOAD-BEARING test: it goes RED if the
// `if w == 0 { continue }` guard is removed from weightedShuffleEndpoints — without
// the guard, m2-excluded enters tryOrder, is reached after m1+m3 fail, and the
// attempted-model assertion fires.
func TestWeightedSelection_WeightZeroNeverAttempted(t *testing.T) {
	// Both positive-weight servers fail (503 retryable) so the try-loop exhausts
	// all tryOrder entries. If m2-excluded were in tryOrder (guard absent), it
	// would be reached and appear in the attempted list.
	failM1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failM1.Close()

	// m2-excluded: if contacted, the test must fail immediately.
	excludedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("weight-0 model m2-excluded was attempted; structural exclusion guard broken")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer excludedSrv.Close()

	failM3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failM3.Close()

	rng := rand.New(rand.NewSource(42))
	var attempted []string
	obs := func(ep llm.Endpoint, _ error) {
		attempted = append(attempted, ep.Model)
	}

	for range 50 {
		attempted = attempted[:0]
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: failM1.URL, Key: "k", Model: "m1"},
				{URL: excludedSrv.URL, Key: "k", Model: "m2-excluded"},
				{URL: failM3.URL, Key: "k", Model: "m3"},
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionWeighted),
			llm.WithModelWeights(map[string]int{"m1": 4, "m2-excluded": 0, "m3": 1}),
			llm.WithRander(rng),
			llm.WithEndpointAttemptObserver(obs),
		)
		// Call will fail (all positive-weight models return 503) — that is expected.
		_, _ = c.Complete(context.Background(), "", "test")

		for _, m := range attempted {
			if m == "m2-excluded" {
				t.Errorf("weight-0 model m2-excluded was attempted; structural exclusion guard broken; attempts=%v", attempted)
				return
			}
		}
	}
}

// TestWeightedSelection_FallbackWorks verifies that when the highest-weight model
// fails with 503 (retryable), the chain advances to the lower-weight model.
// m2 (weight=0) is excluded; m3 (weight=1) must serve the request.
//
// Mutation proof: if the `if w == 0 { continue }` guard is removed, m2-excluded
// enters tryOrder at key=0 (sorts last after m1 and m3). m1 fails → m3 serves →
// m2 unreached in this scenario. The weight-0 exclusion is mutation-proven by
// TestWeightedSelection_WeightZeroNeverAttempted (which makes m3 also fail, forcing
// fallthrough to m2 if the guard is absent). This test proves the fallback advance
// mechanism: the chain correctly skips a failed endpoint and tries the next.
func TestWeightedSelection_FallbackWorks(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()
	// m2 excluded server — the handler fires t.Error if contacted.
	excludedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("weight-0 model m2 was attempted")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer excludedSrv.Close()
	okSrv := httptest.NewServer(okChatHandler(t, "m3-served"))
	defer okSrv.Close()

	rng := rand.New(rand.NewSource(42))
	var attempted []string
	obs := func(ep llm.Endpoint, _ error) {
		attempted = append(attempted, ep.Model)
	}
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: failSrv.URL, Key: "k", Model: "m1"},
			{URL: excludedSrv.URL, Key: "k", Model: "m2"},
			{URL: okSrv.URL, Key: "k", Model: "m3"},
		}),
		llm.WithMaxRetries(1),
		llm.WithSelectionStrategy(llm.SelectionWeighted),
		llm.WithModelWeights(map[string]int{"m1": 4, "m2": 0, "m3": 1}),
		llm.WithRander(rng),
		llm.WithEndpointAttemptObserver(obs),
	)

	resp, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("expected fallback success, got error: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty response from fallback m3")
	}
	// Verify chain advanced past m1 (fallback worked) and m2 was excluded.
	m1seen, m3seen := false, false
	for _, m := range attempted {
		if m == "m2" {
			t.Errorf("weight-0 model m2 appeared in attempts; guard broken: %v", attempted)
		}
		if m == "m1" {
			m1seen = true
		}
		if m == "m3" {
			m3seen = true
		}
	}
	if !m1seen {
		t.Errorf("m1 (failing) never attempted; chain did not start: %v", attempted)
	}
	if !m3seen {
		t.Errorf("m3 (fallback) never attempted; chain did not advance: %v", attempted)
	}
}

// TestWeightedSelection_CooledHighWeightSkipped asserts that a cooled model with
// high weight is not selected. m1 (weight=10) gets cooled; m2 (weight=1) must
// serve all subsequent requests.
func TestWeightedSelection_CooledHighWeightSkipped(t *testing.T) {
	quota429Body := `{"error":{"message":"quota","type":"rate_limit_exceeded"}}`
	quotaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(quota429Body))
	}))
	defer quotaSrv.Close()
	okSrv := httptest.NewServer(okChatHandler(t, "m2-healthy"))
	defer okSrv.Close()

	var attempted []string
	obs := func(ep llm.Endpoint, _ error) {
		attempted = append(attempted, ep.Model)
	}

	rng := rand.New(rand.NewSource(42))
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: quotaSrv.URL, Key: "k", Model: "m1-heavy"},
			{URL: okSrv.URL, Key: "k", Model: "m2-healthy"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
		llm.WithSelectionStrategy(llm.SelectionWeighted),
		llm.WithModelWeights(map[string]int{"m1-heavy": 10, "m2-healthy": 1}),
		llm.WithRander(rng),
		llm.WithEndpointAttemptObserver(obs),
	)

	// Phase 1: drive m1-heavy into cooldown.
	const maxWarmup = 10
	cooled := false
	for range maxWarmup {
		attempted = attempted[:0]
		_, _ = c.Complete(context.Background(), "", "test")
		for _, m := range attempted {
			if m == "m1-heavy" {
				cooled = true
			}
		}
		if cooled {
			break
		}
	}
	if !cooled {
		t.Skip("m1-heavy never attempted in warmup; seed produced degenerate sequence")
	}

	// Phase 2: verify m1-heavy never attempted now that it's cooled.
	attempted = attempted[:0]
	for range 10 {
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("post-cooldown call unexpected error: %v", err)
		}
	}
	for _, m := range attempted {
		if m == "m1-heavy" {
			t.Errorf("cooled m1-heavy (weight=10) was attempted; cooldown+weighted exclusion broken: %v", attempted)
			return
		}
	}
	// Sanity: m2-healthy must have been tried.
	seen := false
	for _, m := range attempted {
		if m == "m2-healthy" {
			seen = true
		}
	}
	if !seen {
		t.Errorf("m2-healthy never attempted in post-cooldown calls: %v", attempted)
	}
}

// TestWeightedSelection_AllWeightZeroGuard asserts that when all models have weight=0,
// the call returns a real error (not nil,nil) and does not panic. The race guard
// (endpoints[0] from cooldownCandidates) must fire.
func TestWeightedSelection_AllWeightZeroGuard(t *testing.T) {
	// Return 503 so the call gets a real error back (not a success).
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer errSrv.Close()

	rng := rand.New(rand.NewSource(42))
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: errSrv.URL, Key: "k", Model: "m1"},
			{URL: errSrv.URL, Key: "k", Model: "m2"},
		}),
		llm.WithMaxRetries(1),
		llm.WithSelectionStrategy(llm.SelectionWeighted),
		llm.WithModelWeights(map[string]int{"m1": 0, "m2": 0}),
		llm.WithRander(rng),
	)

	// Must not panic; must return a real error (the 503 from the race guard attempt).
	resp, err := c.Complete(context.Background(), "", "test")
	// Either we get an error or a response — never (nil, nil).
	if resp == "" && err == nil {
		t.Error("all-weight-0 guard: got (nil, nil) — race guard not firing")
	}
	// Expect an error since our server returns 503.
	if err == nil {
		t.Error("expected an error from 503 server, got nil")
	}
}

// TestWeightedSelection_UnlistedModelDefaultWeight1 asserts that models not in
// the weights map get default weight 1 and participate in selection.
func TestWeightedSelection_UnlistedModelDefaultWeight1(t *testing.T) {
	s1 := httptest.NewServer(okChatHandler(t, "m1"))
	defer s1.Close()
	s2 := httptest.NewServer(okChatHandler(t, "m2"))
	defer s2.Close()

	rng := rand.New(rand.NewSource(42))
	firstPicks := make(map[string]int)

	for range 1000 {
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
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionWeighted),
			// m1 not listed → default weight 1; m2 explicit weight 4.
			llm.WithModelWeights(map[string]int{"m2": 4}),
			llm.WithRander(rng),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if firstModel != "" {
			firstPicks[firstModel]++
		}
	}

	// m2 should be first more often (weight=4 vs weight=1 default).
	if firstPicks["m2"] <= firstPicks["m1"] {
		t.Errorf("m2 (weight=4) should dominate m1 (default weight 1); got m1=%d m2=%d", firstPicks["m1"], firstPicks["m2"])
	}
	// m1 must still appear (default weight 1, not excluded).
	if firstPicks["m1"] == 0 {
		t.Errorf("m1 (unlisted, default weight 1) never picked first; must participate: dist=%v", firstPicks)
	}
}

// TestWeightedSelection_EnvParsing asserts that parseModelWeights correctly
// handles valid pairs, skips malformed ones, accepts weight=0, and skips negatives.
func TestWeightedSelection_EnvParsing(t *testing.T) {
	// "a:4,b:0,bad,c:notnum,d:-1,e:2"
	// Expected: a=4, b=0, e=2; bad/c/d skipped.
	got := llm.ParseModelWeights("a:4,b:0,bad,c:notnum,d:-1,e:2")
	cases := []struct {
		key  string
		want int
		ok   bool
	}{
		{"a", 4, true},
		{"b", 0, true},
		{"e", 2, true},
	}
	for _, tc := range cases {
		v, exists := got[tc.key]
		if !exists {
			t.Errorf("key %q missing from result (want %d); full map=%v", tc.key, tc.want, got)
			continue
		}
		if v != tc.want {
			t.Errorf("key %q = %d, want %d", tc.key, v, tc.want)
		}
	}
	// Skipped entries must not appear.
	for _, bad := range []string{"bad", "c", "d"} {
		if _, exists := got[bad]; exists {
			t.Errorf("invalid key %q should be skipped, but exists in map=%v", bad, got)
		}
	}
	// Empty string → nil.
	if llm.ParseModelWeights("") != nil {
		t.Errorf("ParseModelWeights(\"\") should return nil")
	}
}

// TestWithModelWeights_NegativeWeightExcluded asserts that WithModelWeights
// skips negative weights (same contract as parseModelWeights) rather than
// promoting the model (math.Pow(rf, 1/negative) > 1 > all positive keys).
// Without this guard a negative-weight model sorts FIRST — the opposite of
// suppression.
func TestWithModelWeights_NegativeWeightExcluded(t *testing.T) {
	// bad-model (weight=-1) should never be first-picked.
	// If the negative weight passes through: key = rf^(1/-1) = rf^(-1) > 1
	// which always sorts higher than any positive-weight model's key ∈ (0,1).
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("negative-weight model was promoted to first-pick and attempted")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badSrv.Close()
	goodSrv := httptest.NewServer(okChatHandler(t, "good"))
	defer goodSrv.Close()

	rng := rand.New(rand.NewSource(42))
	for range 50 {
		var firstModel string
		obs := func(ep llm.Endpoint, err error) {
			if err == nil && firstModel == "" {
				firstModel = ep.Model
			}
		}
		c := llm.NewClient("", "", "",
			llm.WithEndpoints([]llm.Endpoint{
				{URL: badSrv.URL, Key: "k", Model: "bad"},
				{URL: goodSrv.URL, Key: "k", Model: "good"},
			}),
			llm.WithMaxRetries(1),
			llm.WithSelectionStrategy(llm.SelectionWeighted),
			llm.WithModelWeights(map[string]int{"bad": -1, "good": 1}),
			llm.WithRander(rng),
			llm.WithEndpointAttemptObserver(obs),
		)
		_, err := c.Complete(context.Background(), "", "test")
		if err != nil {
			t.Fatalf("unexpected error (good server should serve): %v", err)
		}
	}
}