package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
)

// modelsServer returns an httptest server that serves the given model ids at
// /v1/models in the OpenAI-compatible shape, and counts how many times the
// /v1/models route was hit (to assert TTL cache behaviour). rawBody, when
// non-empty, is served verbatim instead of the generated JSON (for malformed
// / non-data-shaped bodies); status overrides 200 when non-zero.
func modelsServer(t *testing.T, ids []string, rawBody string, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		if status != 0 {
			w.WriteHeader(status)
		}
		if rawBody != "" {
			_, _ = w.Write([]byte(rawBody))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsJSON(ids)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func modelsJSON(ids []string) string {
	out := `{"data":[`
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += `{"id":"` + id + `"}`
	}
	out += `]}`
	return out
}

func models(eps []llm.Endpoint) []string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = e.Model
	}
	return out
}

// (a) dead models filtered, order preserved.
func TestBuildModelChainEndpointsFiltered_DropsDeadPreservesOrder(t *testing.T) {
	// live set is missing "dead-1" and "dead-2"; "a","b","c" survive in order.
	srv, _ := modelsServer(t, []string{"c", "a", "b", "other"}, "", 0)
	reg := llm.NewModelRegistry()

	var ev llm.ModelFilterEvent
	obs := func(e llm.ModelFilterEvent) { ev = e }

	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, "k",
		"a", []string{"dead-1", "b", "dead-2", "c"}, obs,
	)

	wantModels := []string{"a", "b", "c"}
	if g := models(got); !reflect.DeepEqual(g, wantModels) {
		t.Fatalf("models = %v, want %v (order must be preserved)", g, wantModels)
	}
	// Every kept endpoint carries baseURL + key from the builder.
	for _, e := range got {
		if e.Key != "k" {
			t.Errorf("endpoint %q lost its key: %+v", e.Model, e)
		}
	}
	if ev.Degraded {
		t.Errorf("event should not be degraded: %+v", ev)
	}
	// Requested = primary "a" + 4 chain entries = 5 endpoints; 3 survive.
	if ev.Kept != 3 || ev.Requested != 5 {
		t.Errorf("event Kept/Requested = %d/%d, want 3/5: %+v", ev.Kept, ev.Requested, ev)
	}
	wantDropped := []string{"dead-1", "dead-2"}
	if !reflect.DeepEqual(ev.Dropped, wantDropped) {
		t.Errorf("event Dropped = %v, want %v (chain order)", ev.Dropped, wantDropped)
	}
	if ev.Available != 4 {
		t.Errorf("event Available = %d, want 4", ev.Available)
	}
}

// (b) /v1/models down → full chain returned (graceful), event degraded.
func TestBuildModelChainEndpointsFiltered_ModelsDown_FullChainGraceful(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string   // when set, used directly (unreachable case)
		ids        []string // server-served ids (when server used)
		rawBody    string   // malformed body
		status     int      // non-200 status
		wantReason string
	}{
		{name: "unreachable", baseURL: "http://127.0.0.1:1/", wantReason: "fetch_failed"},
		{name: "non-200", status: http.StatusInternalServerError, wantReason: "fetch_failed"},
		{name: "malformed json", rawBody: "}{not json", wantReason: "fetch_failed"},
		{name: "empty data set", ids: []string{}, wantReason: "empty_set"},
		{name: "garbage shape no data", rawBody: `{"models":["x"]}`, wantReason: "empty_set"},
	}
	chain := []string{"b", "c"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL := tt.baseURL
			if baseURL == "" {
				srv, _ := modelsServer(t, tt.ids, tt.rawBody, tt.status)
				baseURL = srv.URL
			}
			reg := llm.NewModelRegistry()
			var ev llm.ModelFilterEvent
			got := llm.BuildModelChainEndpointsFiltered(
				context.Background(), reg, baseURL, "k", "a", chain,
				func(e llm.ModelFilterEvent) { ev = e },
			)
			// Full unfiltered chain — today's behaviour, never an empty result.
			wantModels := []string{"a", "b", "c"}
			if g := models(got); !reflect.DeepEqual(g, wantModels) {
				t.Fatalf("models = %v, want full unfiltered %v", g, wantModels)
			}
			if !ev.Degraded {
				t.Errorf("event must be Degraded on %s: %+v", tt.name, ev)
			}
			if ev.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", ev.Reason, tt.wantReason)
			}
			if ev.Kept != len(wantModels) {
				t.Errorf("Kept = %d, want %d (full chain)", ev.Kept, len(wantModels))
			}
		})
	}
}

// nil registry → full chain, no_registry reason.
func TestBuildModelChainEndpointsFiltered_NilRegistry_FullChain(t *testing.T) {
	var ev llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), nil, "http://x/v1", "k", "a", []string{"b"},
		func(e llm.ModelFilterEvent) { ev = e },
	)
	if g := models(got); !reflect.DeepEqual(g, []string{"a", "b"}) {
		t.Fatalf("models = %v, want [a b]", g)
	}
	if !ev.Degraded || ev.Reason != "no_registry" {
		t.Errorf("want Degraded no_registry, got %+v", ev)
	}
}

// nil observer must not panic.
func TestBuildModelChainEndpointsFiltered_NilObserver_NoPanic(t *testing.T) {
	srv, _ := modelsServer(t, []string{"a"}, "", 0)
	reg := llm.NewModelRegistry()
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, "k", "a", []string{"dead"}, nil,
	)
	if g := models(got); !reflect.DeepEqual(g, []string{"a"}) {
		t.Fatalf("models = %v, want [a]", g)
	}
}

// (c) all-filtered → unfiltered fallback + degraded all_filtered warning.
func TestBuildModelChainEndpointsFiltered_AllFiltered_UnfilteredFallback(t *testing.T) {
	// live set shares NOTHING with the chain → filtering would empty it.
	srv, _ := modelsServer(t, []string{"x", "y", "z"}, "", 0)
	reg := llm.NewModelRegistry()
	var ev llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, "k", "a", []string{"b", "c"},
		func(e llm.ModelFilterEvent) { ev = e },
	)
	// Must NOT be empty — degrade to full chain.
	wantModels := []string{"a", "b", "c"}
	if g := models(got); !reflect.DeepEqual(g, wantModels) {
		t.Fatalf("models = %v, want full unfiltered %v (never empty)", g, wantModels)
	}
	if !ev.Degraded || ev.Reason != "all_filtered" {
		t.Errorf("want Degraded all_filtered, got %+v", ev)
	}
	// all_filtered still reports what it would have dropped + the live set size.
	if ev.Available != 3 {
		t.Errorf("Available = %d, want 3", ev.Available)
	}
	if !reflect.DeepEqual(ev.Dropped, []string{"a", "b", "c"}) {
		t.Errorf("Dropped = %v, want [a b c]", ev.Dropped)
	}
}

// (d) TTL cache hit — second call does not re-fetch /v1/models.
func TestModelRegistry_TTLCacheHit_NoRefetch(t *testing.T) {
	srv, hits := modelsServer(t, []string{"a", "b"}, "", 0)
	reg := llm.NewModelRegistry() // default 5m TTL

	for i := 0; i < 3; i++ {
		_ = llm.BuildModelChainEndpointsFiltered(
			context.Background(), reg, srv.URL, "k", "a", []string{"b"}, nil,
		)
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("/v1/models hit %d times across 3 calls, want 1 (TTL cache)", got)
	}
}

// TTL expiry forces a re-fetch.
func TestModelRegistry_TTLExpiry_Refetches(t *testing.T) {
	srv, hits := modelsServer(t, []string{"a"}, "", 0)
	reg := llm.NewModelRegistry(llm.WithModelRegistryTTL(20 * time.Millisecond))

	_ = llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL, "k", "a", nil, nil)
	time.Sleep(40 * time.Millisecond)
	_ = llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL, "k", "a", nil, nil)

	if got := atomic.LoadInt32(hits); got != 2 {
		t.Fatalf("/v1/models hit %d times, want 2 (TTL expired between calls)", got)
	}
}

// A failed fetch must NOT be cached — the next call retries.
func TestModelRegistry_FailedFetchNotCached_Retries(t *testing.T) {
	var hits int32
	var fail atomic.Bool
	fail.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(modelsJSON([]string{"a", "b"})))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	reg := llm.NewModelRegistry()

	// First call: server fails → degrade, nothing cached.
	var ev1 llm.ModelFilterEvent
	_ = llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL, "k", "a",
		[]string{"b"}, func(e llm.ModelFilterEvent) { ev1 = e })
	if !ev1.Degraded {
		t.Fatalf("first call should degrade on 500: %+v", ev1)
	}

	// Recover the server; second call must re-fetch and now filter.
	fail.Store(false)
	var ev2 llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL, "k", "a",
		[]string{"b", "dead"}, func(e llm.ModelFilterEvent) { ev2 = e })
	if ev2.Degraded {
		t.Fatalf("second call should succeed after recovery: %+v", ev2)
	}
	if g := models(got); !reflect.DeepEqual(g, []string{"a", "b"}) {
		t.Errorf("models = %v, want [a b] (dead dropped)", g)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2 (failed fetch not cached → retried)", got)
	}
}

// baseURL normalization: "...:port" and "...:port/v1" share one cache entry.
func TestModelRegistry_BaseURLNormalization_SharesCache(t *testing.T) {
	srv, hits := modelsServer(t, []string{"a"}, "", 0)
	reg := llm.NewModelRegistry()

	_ = llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL, "k", "a", nil, nil)
	_ = llm.BuildModelChainEndpointsFiltered(context.Background(), reg, srv.URL+"/v1", "k", "a", nil, nil)

	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("hits = %d, want 1 (baseURL and baseURL/v1 must share a cache entry)", got)
	}
}

// Concurrency: many goroutines hitting a cold registry collapse to one fetch
// (and -race must stay clean).
func TestModelRegistry_ConcurrentColdStart_SingleFetch(t *testing.T) {
	srv, hits := modelsServer(t, []string{"a", "b"}, "", 0)
	reg := llm.NewModelRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = llm.BuildModelChainEndpointsFiltered(
				context.Background(), reg, srv.URL, "k", "a", []string{"b"}, nil,
			)
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Fatalf("/v1/models hit %d times under 32 concurrent cold callers, want 1", got)
	}
}


// ---------------------------------------------------------------------------
// Auth header tests (TDD: written RED before the fix).
// ---------------------------------------------------------------------------

// TestModelRegistry_AuthHeader_Sent verifies that fetch sends Authorization: Bearer <apiKey>
// when apiKey is non-empty. Without the fix, available() does not accept apiKey
// and the request goes out without auth -> the server 401s -> ok=false.
func TestModelRegistry_AuthHeader_Sent(t *testing.T) {
	const wantKey = "test-key"
	var gotAuthHeader string
	var headerMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		headerMu.Lock()
		gotAuthHeader = auth
		headerMu.Unlock()
		if auth != "Bearer "+wantKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsJSON([]string{"gpt-4o"})))
	}))
	t.Cleanup(srv.Close)

	reg := llm.NewModelRegistry()
	var ev llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, wantKey,
		"gpt-4o", []string{}, func(e llm.ModelFilterEvent) { ev = e },
	)

	// Model must survive (auth -> 200 -> "gpt-4o" in live set).
	if g := models(got); len(g) == 0 || g[0] != "gpt-4o" {
		t.Fatalf("models = %v, want [gpt-4o] -- auth header not sent", g)
	}
	if ev.Degraded {
		t.Errorf("event must not be degraded when auth succeeds: %+v", ev)
	}

	headerMu.Lock()
	h := gotAuthHeader
	headerMu.Unlock()
	if h != "Bearer "+wantKey {
		t.Errorf("server received Authorization = %q, want %q", h, "Bearer "+wantKey)
	}
}

// TestModelRegistry_NoAuth_Degraded verifies that an empty apiKey -> 401 -> graceful
// degradation (full chain returned, no panic).
func TestModelRegistry_NoAuth_Degraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsJSON([]string{"gpt-4o"})))
	}))
	t.Cleanup(srv.Close)

	reg := llm.NewModelRegistry()
	var ev llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, "", // empty apiKey
		"gpt-4o", []string{"gpt-4"}, func(e llm.ModelFilterEvent) { ev = e },
	)

	// Graceful: full chain returned, degraded.
	wantModels := []string{"gpt-4o", "gpt-4"}
	if g := models(got); !reflect.DeepEqual(g, wantModels) {
		t.Fatalf("models = %v, want full chain %v (graceful degradation)", g, wantModels)
	}
	if !ev.Degraded {
		t.Errorf("event must be degraded on 401: %+v", ev)
	}
	if ev.Reason != "fetch_failed" {
		t.Errorf("Reason = %q, want fetch_failed", ev.Reason)
	}
}

// TestBuildModelChainEndpointsFiltered_DropsAbsentModel is an end-to-end auth+filter
// test: auth reaches the wire AND absent model is dropped from the chain.
func TestBuildModelChainEndpointsFiltered_DropsAbsentModel(t *testing.T) {
	const wantKey = "test-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+wantKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Only gpt-4o is live; gpt-4 is absent.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(modelsJSON([]string{"gpt-4o"})))
	}))
	t.Cleanup(srv.Close)

	reg := llm.NewModelRegistry()
	var ev llm.ModelFilterEvent
	got := llm.BuildModelChainEndpointsFiltered(
		context.Background(), reg, srv.URL, wantKey,
		"gpt-4o", []string{"gpt-4"}, func(e llm.ModelFilterEvent) { ev = e },
	)

	// gpt-4o present -> kept; gpt-4 absent -> dropped.
	if g := models(got); len(g) != 1 || g[0] != "gpt-4o" {
		t.Fatalf("models = %v, want [gpt-4o] only -- gpt-4 must be dropped", g)
	}
	if ev.Degraded {
		t.Errorf("must not degrade when filter works: %+v", ev)
	}
	if !reflect.DeepEqual(ev.Dropped, []string{"gpt-4"}) {
		t.Errorf("Dropped = %v, want [gpt-4]", ev.Dropped)
	}
}
