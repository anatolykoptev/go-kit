package sparse

import (
	"context"
	"sync"
	"testing"
)

// memCache is a trivial in-memory SparseCache for tests.
type memCache struct {
	mu sync.Mutex
	m  map[string]SparseVector
}

func newMemCache() *memCache { return &memCache{m: make(map[string]SparseVector)} }

func (c *memCache) Get(_ context.Context, key string) (SparseVector, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[key]
	return v, ok
}
func (c *memCache) Set(_ context.Context, key string, v SparseVector) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = v
}

// TestCacheKey_StableAcrossCalls verifies cacheKey is deterministic.
func TestCacheKey_StableAcrossCalls(t *testing.T) {
	a := cacheKey("m", 256, 0.0, 30522, "hello")
	b := cacheKey("m", 256, 0.0, 30522, "hello")
	if a != b {
		t.Errorf("non-deterministic: %s vs %s", a, b)
	}
}

// TestCacheKey_DistinguishesAllInputs verifies that flipping any
// vector-affecting parameter produces a different cache key.
func TestCacheKey_DistinguishesAllInputs(t *testing.T) {
	base := cacheKey("m", 256, 0.0, 30522, "hello")
	cases := []struct {
		name string
		key  string
	}{
		{"model", cacheKey("n", 256, 0.0, 30522, "hello")},
		{"top_k", cacheKey("m", 128, 0.0, 30522, "hello")},
		{"min_weight", cacheKey("m", 256, 0.1, 30522, "hello")},
		{"vocab_size", cacheKey("m", 256, 0.0, 50000, "hello")},
		{"text", cacheKey("m", 256, 0.0, 30522, "world")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.key == base {
				t.Errorf("expected key to differ when %s changes", tc.name)
			}
		})
	}
}

// TestCacheKey_Format verifies the key is hex-encoded and 64 chars.
func TestCacheKey_Format(t *testing.T) {
	k := cacheKey("m", 256, 0.0, 30522, "hello")
	if len(k) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d (%s)", len(k), k)
	}
}

// TestTryCacheFullBatchGet_AllHit returns vectors in input order.
func TestTryCacheFullBatchGet_AllHit(t *testing.T) {
	c := newMemCache()
	ctx := context.Background()
	v1 := SparseVector{Indices: []uint32{1}, Values: []float32{0.1}}
	v2 := SparseVector{Indices: []uint32{2}, Values: []float32{0.2}}
	c.Set(ctx, cacheKey("m", 0, 0, 30522, "a"), v1)
	c.Set(ctx, cacheKey("m", 0, 0, 30522, "b"), v2)

	got := tryCacheFullBatchGet(ctx, c, "m", 0, 0, 30522, []string{"a", "b"})
	if got == nil {
		t.Fatal("expected hit, got nil")
	}
	if got[0].Indices[0] != 1 || got[1].Indices[0] != 2 {
		t.Errorf("wrong order: %+v", got)
	}
}

// TestTryCacheFullBatchGet_PartialMiss returns nil so the caller falls
// through to the backend for the full batch.
func TestTryCacheFullBatchGet_PartialMiss(t *testing.T) {
	c := newMemCache()
	ctx := context.Background()
	c.Set(ctx, cacheKey("m", 0, 0, 30522, "a"), SparseVector{Indices: []uint32{1}, Values: []float32{0.1}})

	got := tryCacheFullBatchGet(ctx, c, "m", 0, 0, 30522, []string{"a", "b"})
	if got != nil {
		t.Errorf("expected nil on partial miss, got %v", got)
	}
}

// TestFtoa_DistinctFloatsDistinctStrings sanity-checks the bit-pattern
// helper.
func TestFtoa_DistinctFloatsDistinctStrings(t *testing.T) {
	if ftoa(0.0) == ftoa(0.1) {
		t.Errorf("0.0 and 0.1 collide: %s", ftoa(0.0))
	}
	if ftoa(0.1) == ftoa(0.2) {
		t.Errorf("0.1 and 0.2 collide")
	}
	if ftoa(0.0) != ftoa(0.0) {
		t.Errorf("non-deterministic")
	}
}
