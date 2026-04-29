package embed

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
	m  map[string][]float32
}

func newMapCache() *mapCache {
	return &mapCache{m: make(map[string][]float32)}
}

func (c *mapCache) Get(_ context.Context, k string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}

func (c *mapCache) Set(_ context.Context, k string, v []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
}

// ── cacheKey tests ────────────────────────────────────────────────────────────

func TestCacheKey_Deterministic(t *testing.T) {
	k1 := cacheKey("model-a", 1024, "passage: ", "query: ", "hello world")
	k2 := cacheKey("model-a", 1024, "passage: ", "query: ", "hello world")
	if k1 != k2 {
		t.Errorf("cacheKey not deterministic: %q != %q", k1, k2)
	}
	if len(k1) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got len %d", len(k1))
	}
}

func TestCacheKey_DifferentModel_DifferentKey(t *testing.T) {
	k1 := cacheKey("model-a", 1024, "", "", "text")
	k2 := cacheKey("model-b", 1024, "", "", "text")
	if k1 == k2 {
		t.Error("different models must produce different cache keys")
	}
}

func TestCacheKey_DifferentDim_DifferentKey(t *testing.T) {
	// Matryoshka truncation (E4 prep): dim=512 vs dim=1024 must differ.
	k1 := cacheKey("model", 512, "", "", "text")
	k2 := cacheKey("model", 1024, "", "", "text")
	if k1 == k2 {
		t.Error("different dim must produce different cache keys (Matryoshka E4 prep)")
	}
}

func TestCacheKey_DifferentDocPrefix_DifferentKey(t *testing.T) {
	k1 := cacheKey("model", 1024, "passage: ", "", "text")
	k2 := cacheKey("model", 1024, "query: ", "", "text")
	if k1 == k2 {
		t.Error("different docPrefix must produce different cache keys")
	}
}

func TestCacheKey_DifferentText_DifferentKey(t *testing.T) {
	k1 := cacheKey("model", 1024, "", "", "text A")
	k2 := cacheKey("model", 1024, "", "", "text B")
	if k1 == k2 {
		t.Error("different text must produce different cache keys")
	}
}

// ── httptest server helper ────────────────────────────────────────────────────

// cacheTestServer builds a fake embed-server that records calls and returns
// deterministic vectors (vec[i] = [float32(i+1), ...] of dim 4).
func cacheTestServer(t *testing.T, callCount *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		type embeddingObj struct {
			Object    string    `json:"object"`
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		type respBody struct {
			Object string         `json:"object"`
			Data   []embeddingObj `json:"data"`
			Model  string         `json:"model"`
		}
		data := make([]embeddingObj, len(req.Input))
		for i := range req.Input {
			data[i] = embeddingObj{
				Object:    "embedding",
				Embedding: []float32{float32(i + 1), 0, 0, 0},
				Index:     i,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{ //nolint:errcheck
			Object: "list",
			Data:   data,
			Model:  req.Model,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ── integration tests ─────────────────────────────────────────────────────────

func TestCache_FullBatchHit_SkipsBackend(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	texts := []string{"alpha", "beta", "gamma"}
	cache := newMapCache()
	model := "test-model"
	ctx := context.Background()

	// Pre-populate cache for all 3 texts (dim=4, no prefixes).
	for i, txt := range texts {
		cache.Set(ctx, cacheKey(model, 4, "", "", txt), []float32{float32(i + 1), 0, 0, 0})
	}

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel(model),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(cache),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	res, err := c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 0 {
		t.Errorf("backend called %d times; want 0 (full-batch cache hit)", callCount.Load())
	}
	if len(res.Vectors) != 3 {
		t.Fatalf("vectors len: got %d want 3", len(res.Vectors))
	}
}

func TestCache_PartialMiss_HitsBackend(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	texts := []string{"alpha", "beta", "gamma"}
	cache := newMapCache()
	model := "test-model"
	ctx := context.Background()

	// Only populate 2 of 3 — partial miss → backend must be called.
	cache.Set(ctx, cacheKey(model, 4, "", "", "alpha"), []float32{1, 0, 0, 0})
	cache.Set(ctx, cacheKey(model, 4, "", "", "beta"), []float32{2, 0, 0, 0})
	// "gamma" is absent.

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel(model),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(cache),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	res, err := c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 1 {
		t.Errorf("backend called %d times; want 1 (partial miss → full batch)", callCount.Load())
	}
}

func TestCache_PopulateAfterBackend(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	texts := []string{"alpha", "beta"}
	cache := newMapCache()
	model := "test-model"

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel(model),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(cache),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()

	// First call: cache empty → backend (count=1) → cache populated.
	res1, err := c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if callCount.Load() != 1 {
		t.Errorf("first call: backend count %d want 1", callCount.Load())
	}
	if res1.Status != StatusOk {
		t.Errorf("first call status: got %v want Ok", res1.Status)
	}

	// Second call: same texts → full-batch cache hit → no backend.
	res2, err := c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if callCount.Load() != 1 {
		t.Errorf("second call: backend count %d want still 1 (cache hit)", callCount.Load())
	}
	if res2.Status != StatusOk {
		t.Errorf("second call status: got %v want Ok", res2.Status)
	}
}

func TestCache_OnCacheHitHookFires(t *testing.T) {
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	texts := []string{"alpha", "beta"}
	cache := newMapCache()
	model := "test-model"
	ctx := context.Background()

	// Pre-populate cache.
	cache.Set(ctx, cacheKey(model, 4, "", "", "alpha"), []float32{1, 0, 0, 0})
	cache.Set(ctx, cacheKey(model, 4, "", "", "beta"), []float32{2, 0, 0, 0})

	obs := &countingObserver{}
	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel(model),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(cache),
		WithObserver(obs),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obs.cacheHit != 1 {
		t.Errorf("OnCacheHit: want 1 call, got %d", obs.cacheHit)
	}
	if callCount.Load() != 0 {
		t.Errorf("backend called %d times; want 0 on cache hit", callCount.Load())
	}
}

func TestCache_NilCache_NoOp(t *testing.T) {
	// Passing nil Cache to WithCache should be a no-op — backend still called normally.
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel("test"),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(nil), // should be ignored
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	res, err := c.EmbedWithResult(context.Background(), []string{"alpha"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if callCount.Load() != 1 {
		t.Errorf("backend called %d times; want 1 (nil cache is no-op)", callCount.Load())
	}
}

func TestMetrics_CacheHitMissAreEvents(t *testing.T) {
	// hit_total and miss_total count REQUEST events — not per-text.
	// 1 hit request + 1 miss request → +1 hit, +1 miss (not += N*texts).
	var callCount atomic.Int32
	srv := cacheTestServer(t, &callCount)

	model := "cache-event-metrics-model"
	texts := []string{"x", "y"}
	cache := newMapCache()
	ctx := context.Background()

	c, err := NewClient(srv.URL,
		WithBackend("http"),
		WithModel(model),
		WithDim(4),
		WithTimeout(time.Second),
		WithCache(cache),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	hitBefore := counterValue(embedCacheHitTotal.WithLabelValues(model))
	missBefore := counterValue(embedCacheMissTotal.WithLabelValues(model))

	// Miss: cache empty → backend.
	_, err = c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("miss request error: %v", err)
	}

	// Hit: cache now populated.
	_, err = c.EmbedWithResult(ctx, texts)
	if err != nil {
		t.Fatalf("hit request error: %v", err)
	}

	hitAfter := counterValue(embedCacheHitTotal.WithLabelValues(model))
	missAfter := counterValue(embedCacheMissTotal.WithLabelValues(model))

	if got := hitAfter - hitBefore; got != 1 {
		t.Errorf("cache_hit_total: got delta %.0f want 1 (one event per request)", got)
	}
	if got := missAfter - missBefore; got != 1 {
		t.Errorf("cache_miss_total: got delta %.0f want 1 (one event per request)", got)
	}
}

// counterValue reads the current value of a prometheus.Counter via its Write method.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}
