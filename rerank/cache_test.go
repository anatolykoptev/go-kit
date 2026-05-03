package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ── mapCache stub ─────────────────────────────────────────────────────────────

// mapCache is an in-process Cache stub for tests — no TTL, no eviction.
type mapCache struct {
	mu sync.Mutex
	m  map[string]float32
}

func newMapCache() *mapCache {
	return &mapCache{m: make(map[string]float32)}
}

func (c *mapCache) Get(_ context.Context, k string) (float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.m[k]
	return s, ok
}

func (c *mapCache) Set(_ context.Context, k string, s float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = s
}

// ── cacheKey tests ────────────────────────────────────────────────────────────

func TestCache_KeyDeterministic(t *testing.T) {
	k1 := cacheKey("model-a", "", "", "", "what is Go?", "Go is a language", 0, 0)
	k2 := cacheKey("model-a", "", "", "", "what is Go?", "Go is a language", 0, 0)
	if k1 != k2 {
		t.Errorf("cacheKey not deterministic: %q != %q", k1, k2)
	}
	if len(k1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got len %d", len(k1))
	}
}

func TestCache_KeyDifferent_PerModel(t *testing.T) {
	k1 := cacheKey("model-a", "", "", "", "query", "doc text", 0, 0)
	k2 := cacheKey("model-b", "", "", "", "query", "doc text", 0, 0)
	if k1 == k2 {
		t.Error("different models must produce different cache keys")
	}
}

func TestCache_KeyIncludesModel_ChangesKeyOnModelSwitch(t *testing.T) {
	// Switching model should invalidate cache by producing a different key.
	k1 := cacheKey("bge-base", "", "", "", "test query", "test document", 0, 0)
	k2 := cacheKey("bge-large", "", "", "", "test query", "test document", 0, 0)
	if k1 == k2 {
		t.Error("model change must produce different cache key")
	}
}

func TestCache_KeyDifferent_Query(t *testing.T) {
	k1 := cacheKey("m", "", "", "", "query1", "doc", 0, 0)
	k2 := cacheKey("m", "", "", "", "query2", "doc", 0, 0)
	if k1 == k2 {
		t.Error("different queries must produce different keys")
	}
}

func TestCache_KeyDifferent_Doc(t *testing.T) {
	k1 := cacheKey("m", "", "", "", "q", "doc1", 0, 0)
	k2 := cacheKey("m", "", "", "", "q", "doc2", 0, 0)
	if k1 == k2 {
		t.Error("different docs must produce different keys")
	}
}

func TestCache_KeyChangesWithServerNormalize(t *testing.T) {
	// Same model/query/doc but different serverNormalize must yield different keys.
	k1 := cacheKey("m", "", "", "", "q", "doc", 0, 0)
	k2 := cacheKey("m", "sigmoid", "", "", "q", "doc", 0, 0)
	if k1 == k2 {
		t.Error("different serverNormalize must produce different cache keys")
	}
}

func TestCache_KeyChangesWithInstruction(t *testing.T) {
	// Different queryInstr → different key.
	k1 := cacheKey("m", "", "Represent this query:", "", "q", "doc", 0, 0)
	k2 := cacheKey("m", "", "Represent this sentence:", "", "q", "doc", 0, 0)
	if k1 == k2 {
		t.Error("different queryInstr must produce different cache keys")
	}

	// Different docInstr → different key.
	k3 := cacheKey("m", "", "", "Passage:", "q", "doc", 0, 0)
	k4 := cacheKey("m", "", "", "Document:", "q", "doc", 0, 0)
	if k3 == k4 {
		t.Error("different docInstr must produce different cache keys")
	}

	// Same queryInstr and docInstr → same key.
	k5 := cacheKey("m", "", "q-instr", "d-instr", "q", "doc", 0, 0)
	k6 := cacheKey("m", "", "q-instr", "d-instr", "q", "doc", 0, 0)
	if k5 != k6 {
		t.Error("identical args must produce the same cache key")
	}
}

// ── integration tests with httptest server ────────────────────────────────────

// cacheTestServer creates a test server that records call count and
// returns fixed scores: doc at index i gets score 0.9 - 0.1*i.
func cacheTestServer(t *testing.T, callCount *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req cohereRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		results := make([]cohereResult, len(req.Documents))
		for i := range req.Documents {
			results[i] = cohereResult{
				Index:          i,
				RelevanceScore: 0.9 - float64(i)*0.1,
			}
		}
		v1JSONResp(w, cohereResponse{Model: "test-model", Results: results})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCache_FullBatchHit_SkipsHTTP(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}

	cache := newMapCache()
	model := "test-model"
	query := "test query"

	// Pre-populate cache for all 3 docs (no serverNormalize or instructions).
	ctx := context.Background()
	cache.Set(ctx, cacheKey(model, "", "", "", query, "alpha", 0, 0), 0.9)
	cache.Set(ctx, cacheKey(model, "", "", "", query, "beta", 0, 0), 0.8)
	cache.Set(ctx, cacheKey(model, "", "", "", query, "gamma", 0, 0), 0.7)

	c := NewClient(srv.URL,
		WithModel(model),
		WithTimeout(time.Second),
		WithCache(cache),
	)

	res, err := c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 0 {
		t.Errorf("HTTP called %d times; want 0 (full-batch cache hit)", callCount.Load())
	}
	if len(res.Scored) != 3 {
		t.Fatalf("scored len: got %d want 3", len(res.Scored))
	}
	// Scores should come from cache: a=0.9 > b=0.8 > c=0.7.
	if res.Scored[0].ID != "a" {
		t.Errorf("top doc: got %q want a", res.Scored[0].ID)
	}
}

func TestCache_PartialMiss_HitsHTTP(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
		{ID: "c", Text: "gamma"},
	}

	cache := newMapCache()
	model := "test-model"
	query := "test query"

	// Only populate 2 of 3 docs — should trigger HTTP for full batch.
	ctx := context.Background()
	cache.Set(ctx, cacheKey(model, "", "", "", query, "alpha", 0, 0), 0.9)
	cache.Set(ctx, cacheKey(model, "", "", "", query, "beta", 0, 0), 0.8)
	// "gamma" is absent.

	c := NewClient(srv.URL,
		WithModel(model),
		WithTimeout(time.Second),
		WithCache(cache),
	)

	res, err := c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 1 {
		t.Errorf("HTTP called %d times; want 1 (partial miss → full batch)", callCount.Load())
	}
}

func TestCache_PopulateAfterHTTP(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	docs := []Doc{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "beta"},
	}

	cache := newMapCache()
	model := "test-model"
	query := "test query"

	c := NewClient(srv.URL,
		WithModel(model),
		WithTimeout(time.Second),
		WithCache(cache),
	)

	ctx := context.Background()

	// First call: cache empty → HTTP (count=1) → cache populated.
	res1, err := c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if callCount.Load() != 1 {
		t.Errorf("first call: HTTP count %d want 1", callCount.Load())
	}
	_ = res1

	// Second call: same input → full-batch cache hit → no HTTP.
	res2, err := c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if callCount.Load() != 1 {
		t.Errorf("second call: HTTP count %d want still 1 (cache hit)", callCount.Load())
	}
	if res2.Status != StatusOk {
		t.Errorf("second call status: got %v want Ok", res2.Status)
	}
}

func TestCache_NilCache_NoOp(t *testing.T) {
	// Passing a nil Cache to WithCache should be a no-op (no panic, normal HTTP).
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	docs := []Doc{{ID: "a", Text: "alpha"}}
	c := NewClient(srv.URL,
		WithModel("test-model"),
		WithTimeout(time.Second),
		WithCache(nil), // should be ignored
	)

	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 1 {
		t.Errorf("HTTP count %d want 1 (nil cache is no-op)", callCount.Load())
	}
}

func TestMetrics_CacheHitMissAreEvents(t *testing.T) {
	// Verify that hit_total and miss_total count REQUEST events (not docs).
	// 1 hit request and 1 miss request → hit_total += 1, miss_total += 1 (not += N*docs).
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	model := "metrics-event-model"
	query := "event query"
	docs := []Doc{
		{ID: "x", Text: "xtext"},
		{ID: "y", Text: "ytext"},
	}

	cache := newMapCache()
	ctx := context.Background()

	c := NewClient(srv.URL,
		WithModel(model),
		WithTimeout(time.Second),
		WithCache(cache),
	)

	// Snapshot counters before the test.
	hitBefore := counterValue(rerankCacheHitTotal.WithLabelValues(model))
	missBefore := counterValue(rerankCacheMissTotal.WithLabelValues(model))

	// Miss request: cache empty → HTTP (miss event).
	_, err := c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("miss request error: %v", err)
	}

	// Hit request: cache now populated → no HTTP (hit event).
	_, err = c.RerankWithResult(ctx, query, docs)
	if err != nil {
		t.Fatalf("hit request error: %v", err)
	}

	hitAfter := counterValue(rerankCacheHitTotal.WithLabelValues(model))
	missAfter := counterValue(rerankCacheMissTotal.WithLabelValues(model))

	if got := hitAfter - hitBefore; got != 1 {
		t.Errorf("cache_hit_total: got delta %.0f want 1 (one event per request, not per doc)", got)
	}
	if got := missAfter - missBefore; got != 1 {
		t.Errorf("cache_miss_total: got delta %.0f want 1 (one event per request, not per doc)", got)
	}
}

// counterValue reads the current value of a prometheus.Counter via its Write method.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

// TestRerankCacheKey_TruncationCapAffectsKey verifies that maxCharsPerDoc
// and maxTokensPerDoc are part of the cache key. Without this, bumping a
// truncation cap (e.g. 2000 → 4000 chars) would return stale scores under
// the same key — server truncates BEFORE scoring, so identical docText
// under different caps maps to different inputs to the rerank model.
func TestRerankCacheKey_TruncationCapAffectsKey(t *testing.T) {
	base := func(maxChars, maxTokens int) string {
		return cacheKey("m", "", "", "", "q", "doc", maxChars, maxTokens)
	}
	if base(2000, 0) == base(4000, 0) {
		t.Error("different maxCharsPerDoc must produce different cache keys")
	}
	if base(0, 256) == base(0, 512) {
		t.Error("different maxTokensPerDoc must produce different cache keys")
	}
	if base(2000, 256) == base(4000, 512) {
		t.Error("different (maxChars, maxTokens) tuple must produce different cache keys")
	}
	// Determinism sanity-check.
	if base(2000, 256) != base(2000, 256) {
		t.Error("cacheKey must be deterministic for identical caps")
	}
}
