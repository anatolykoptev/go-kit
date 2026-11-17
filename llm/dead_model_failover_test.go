package llm_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

// Real bodies observed through the fleet proxy (CLIProxyAPI) on 2026-11-16,
// when ~20 free aliases died at once. Verbatim; "..." is part of the captured
// text, not an elision added here.
const (
	openRouterFreeWithdrawn404 = `{"error":{"message":"This model is unavailable for free. The paid version is available now - ..."}}`
	openRouterNoProvider404    = `{"error":{"message":"poolside/laguna-s-2.1:free is no longer available on OpenRouter: no provider ..."}}`
	nvidiaEOL410               = `{"error":{"message":"deepseek-ai/deepseek-v4.1-flash has reached its end of life on 2026-11-16T08:00:00Z"}}`
	zenModelDeprecated410      = `{"error":{"message":"ModelDeprecated: ... use big-pickle instead"}}`
	zenModelUnavailable400     = `{"error":{"message":"Upstream request failed: Model is unavailable."}}`
	unknownProvider502         = `{"error":{"message":"unknown provider for model nv-gpt-oss-120b","type":"server_error","code":"internal_server_error"}}`
	// OpenAI-family 404 model_not_found (param is JSON null).
	openAIModelNotFound404 = `{"error":{"message":"The model ` + "`gpt-5-x`" + ` does not exist or you do not have access to it.","type":"invalid_request_error","param":null,"code":"model_not_found"}}`
	// A wrong base URL: every model answers the same routing 404.
	htmlRouting404 = `<html><head><title>404 Not Found</title></head><body><h1>Not Found</h1></body></html>`
)

// hitCountingHandler serves status+body and counts hits.
func hitCountingHandler(hits *atomic.Int32, status int, body string) http.HandlerFunc {
	h := statusBodyHandler(status, body)
	return func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		h(w, r)
	}
}

// deadThenLive asserts the chain [dead, live] returns live's answer with exactly
// one attempt on the dead endpoint and that the observer saw the dead error.
func deadThenLive(t *testing.T, status int, body string) {
	t.Helper()
	var deadHits atomic.Int32
	dead := httptest.NewServer(hitCountingHandler(&deadHits, status, body))
	defer dead.Close()
	live := httptest.NewServer(okChatHandler(t, "from-live-model"))
	defer live.Close()

	_, calls, obs := newObserver()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "dead-model"},
			{URL: live.URL, Key: "k", Model: "live-model"},
		}),
		llm.WithEndpointAttemptObserver(obs),
	)

	out, err := c.Complete(context.Background(), "", "test")
	if err != nil {
		t.Fatalf("HTTP %d must advance the chain, got error: %v", status, err)
	}
	if out != "from-live-model" {
		t.Errorf("output = %q, want from-live-model", out)
	}
	if got := deadHits.Load(); got != 1 {
		t.Errorf("dead model attempts = %d, want 1", got)
	}
	if len(*calls) != 2 || (*calls)[0].model != "dead-model" || (*calls)[0].err == nil {
		t.Fatalf("observer calls = %+v, want [dead-model err, live-model ok]", *calls)
	}
	var ae *llm.APIError
	if !errors.As((*calls)[0].err, &ae) || ae.StatusCode != status {
		t.Errorf("observed err = %v, want APIError %d", (*calls)[0].err, status)
	}
	if got := llm.ClassifyErrorType((*calls)[0].err); got != "model_unavailable" {
		t.Errorf("ClassifyErrorType = %q, want model_unavailable", got)
	}
}

func TestChain_AdvancesOn404ModelDeath(t *testing.T) {
	for name, body := range map[string]string{
		"openrouter_free_withdrawn": openRouterFreeWithdrawn404,
		"openrouter_no_provider":    openRouterNoProvider404,
		"openai_model_not_found":    openAIModelNotFound404,
	} {
		t.Run(name, func(t *testing.T) { deadThenLive(t, http.StatusNotFound, body) })
	}
}

func TestChain_AdvancesOn410Gone(t *testing.T) {
	for name, body := range map[string]string{
		"empty_body":       "",
		"non_json_body":    "Gone",
		"nvidia_eol":       nvidiaEOL410,
		"zen_deprecated":   zenModelDeprecated410,
		"unrelated_in_410": `{"error":{"message":"whatever"}}`,
	} {
		t.Run(name, func(t *testing.T) { deadThenLive(t, http.StatusGone, body) })
	}
}

func TestChain_AdvancesOn400ModelIsUnavailable(t *testing.T) {
	deadThenLive(t, http.StatusBadRequest, zenModelUnavailable400)
}

// A 404 without a model marker (wrong base URL) recurs on every model: abort on
// the first instead of burning the chain.
func TestChain_404WithoutModelMarkerAborts(t *testing.T) {
	var hits1, hits2 atomic.Int32
	s1 := httptest.NewServer(hitCountingHandler(&hits1, http.StatusNotFound, htmlRouting404))
	defer s1.Close()
	s2 := httptest.NewServer(hitCountingHandler(&hits2, http.StatusNotFound, htmlRouting404))
	defer s2.Close()

	c := llm.NewClient("", "", "", llm.WithEndpoints([]llm.Endpoint{
		{URL: s1.URL, Key: "k", Model: "first"},
		{URL: s2.URL, Key: "k", Model: "second"},
	}))
	if _, err := c.Complete(context.Background(), "", "test"); err == nil {
		t.Fatal("expected a marker-less 404 to abort the chain")
	}
	if hits1.Load() != 1 || hits2.Load() != 0 {
		t.Errorf("hits = (%d, %d), want (1, 0): abort on first", hits1.Load(), hits2.Load())
	}
}

// 502 "unknown provider for model" is a dead alias: one attempt, no same-endpoint
// retries, then the next model. Default maxRetries (3) is kept on purpose.
func TestChain_502UnknownProviderAdvancesWithoutRetry(t *testing.T) {
	var deadHits atomic.Int32
	dead := httptest.NewServer(hitCountingHandler(&deadHits, http.StatusBadGateway, unknownProvider502))
	defer dead.Close()
	live := httptest.NewServer(okChatHandler(t, "from-live-model"))
	defer live.Close()

	c := llm.NewClient("", "", "", llm.WithEndpoints([]llm.Endpoint{
		{URL: dead.URL, Key: "k", Model: "removed-alias"},
		{URL: live.URL, Key: "k", Model: "live-model"},
	}))
	out, err := c.Complete(context.Background(), "", "test")
	if err != nil || out != "from-live-model" {
		t.Fatalf("got (%q, %v), want from-live-model", out, err)
	}
	if got := deadHits.Load(); got != 1 {
		t.Errorf("attempts on dead alias = %d, want 1 (no retries)", got)
	}
}

// Other 502s stay retryable (transient gateway failure).
func TestChain_Plain502StillRetried(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(hitCountingHandler(&hits, http.StatusBadGateway, `{"error":{"message":"bad gateway"}}`))
	defer srv.Close()
	c := llm.NewClient(srv.URL, "k", "m", llm.WithMaxRetries(2))
	if _, err := c.Complete(context.Background(), "", "test"); err == nil {
		t.Fatal("expected error")
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2 (plain 502 retried)", got)
	}
}

// A dead model feeds the per-model cooldown: after FailThreshold (default 2)
// failovers it is skipped entirely on the next request.
func TestCooldown_DeadModelSkippedAfterThreshold(t *testing.T) {
	var deadHits atomic.Int32
	dead := httptest.NewServer(hitCountingHandler(&deadHits, http.StatusGone, ""))
	defer dead.Close()
	live := httptest.NewServer(okChatHandler(t, "from-live-model"))
	defer live.Close()

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "dead-model"},
			{URL: live.URL, Key: "k", Model: "live-model"},
		}),
		llm.WithModelCooldown(llm.CooldownConfig{}), // default FailThreshold = 2
	)
	for i := range 2 {
		if out, err := c.Complete(context.Background(), "", "test"); err != nil || out != "from-live-model" {
			t.Fatalf("call %d: got (%q, %v)", i, out, err)
		}
	}
	if got := deadHits.Load(); got != 2 {
		t.Fatalf("dead hits after 2 calls = %d, want 2", got)
	}
	if out, err := c.Complete(context.Background(), "", "test"); err != nil || out != "from-live-model" {
		t.Fatalf("call 3: got (%q, %v)", out, err)
	}
	if got := deadHits.Load(); got != 2 {
		t.Errorf("dead model hit during cooldown: %d hits, want 2", got)
	}
}

// Request-shaped failovers (413) say nothing about the model: they must not cool.
func TestCooldown_RequestTooLargeDoesNotCool(t *testing.T) {
	var hits atomic.Int32
	big := httptest.NewServer(hitCountingHandler(&hits, http.StatusRequestEntityTooLarge, groq413Body))
	defer big.Close()
	live := httptest.NewServer(okChatHandler(t, "ok"))
	defer live.Close()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: big.URL, Key: "k", Model: "small"},
			{URL: live.URL, Key: "k", Model: "big"},
		}),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)
	for range 3 {
		if _, err := c.Complete(context.Background(), "", "test"); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("413 model hits = %d, want 3 (never cooled)", got)
	}
}

// Degraded > dead: every model dead + cooled still attempts the primary and
// surfaces a real error (never (nil, nil)).
func TestCooldown_AllDeadStillSurfacesError(t *testing.T) {
	var hits atomic.Int32
	dead := httptest.NewServer(hitCountingHandler(&hits, http.StatusGone, ""))
	defer dead.Close()
	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: dead.URL, Key: "k", Model: "a"},
			{URL: dead.URL, Key: "k", Model: "b"},
		}),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
	)
	for i := range 3 {
		out, err := c.Complete(context.Background(), "", "test")
		if err == nil {
			t.Fatalf("call %d: expected error, got %q", i, out)
		}
	}
}
