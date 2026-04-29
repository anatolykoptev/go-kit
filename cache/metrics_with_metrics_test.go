package cache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// gather returns the metric families currently registered on reg, indexed by
// metric family name. Helper for the WithMetrics tests below.
func gather(t *testing.T, reg *prometheus.Registry) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

// findMetric looks up a single sample within an MF by matching all of want's
// labels (subset match — extra labels on the metric are ignored when iterating
// but every want key MUST be present with the want value).
func findMetric(mf *dto.MetricFamily, want map[string]string) *dto.Metric {
	for _, m := range mf.GetMetric() {
		match := true
		for wk, wv := range want {
			found := false
			for _, lp := range m.GetLabel() {
				if lp.GetName() == wk && lp.GetValue() == wv {
					found = true
					break
				}
			}
			if !found {
				match = false
				break
			}
		}
		if match {
			return m
		}
	}
	return nil
}

func TestWithMetrics_RegistersCountersOnce(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "unit"),
	})
	defer c.Close()

	mfs := gather(t, reg)
	for _, want := range []string{
		"gokit_cache_hits_total",
		"gokit_cache_misses_total",
		"gokit_cache_evictions_total",
		"gokit_cache_size",
	} {
		if _, ok := mfs[want]; !ok {
			t.Errorf("metric %q not registered (have: %v)", want, keys(mfs))
		}
	}

	// Each MF must carry the cache=<name> const-label on every sample.
	for name, mf := range mfs {
		if !strings.HasPrefix(name, "gokit_cache_") {
			continue
		}
		for _, m := range mf.GetMetric() {
			if findMetric(mf, map[string]string{"cache": "unit"}) == nil {
				t.Errorf("metric %q sample missing cache=unit label: %v", name, m.GetLabel())
			}
		}
	}
}

func TestWithMetrics_HitMissCounters(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "hm"),
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k1", []byte("v1"))

	// 3 hits.
	for range 3 {
		if _, ok := c.Get(ctx, "k1"); !ok {
			t.Fatal("expected hit")
		}
	}
	// 2 misses.
	for _, k := range []string{"miss1", "miss2"} {
		if _, ok := c.Get(ctx, k); ok {
			t.Fatalf("expected miss for %s", k)
		}
	}

	mfs := gather(t, reg)
	hits := findMetric(mfs["gokit_cache_hits_total"], map[string]string{"cache": "hm", "tier": "L1"})
	if hits == nil {
		t.Fatal("L1 hits metric not found")
	}
	if got := hits.GetCounter().GetValue(); got != 3 {
		t.Errorf("L1 hits = %v, want 3", got)
	}

	misses := findMetric(mfs["gokit_cache_misses_total"], map[string]string{"cache": "hm", "tier": "L1"})
	if misses == nil {
		t.Fatal("L1 misses metric not found")
	}
	if got := misses.GetCounter().GetValue(); got != 2 {
		t.Errorf("L1 misses = %v, want 2", got)
	}

	// L1 size gauge mirrors Stats().L1Size.
	size := findMetric(mfs["gokit_cache_size"], map[string]string{"cache": "hm"})
	if size == nil {
		t.Fatal("size gauge not found")
	}
	if got, want := size.GetGauge().GetValue(), float64(c.Stats().L1Size); got != want {
		t.Errorf("size = %v, want %v", got, want)
	}
}

func TestWithMetrics_DistinctNamesNoConflict(t *testing.T) {
	reg := prometheus.NewRegistry()

	cA := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "A"),
	})
	defer cA.Close()

	// Second cache on the SAME registerer with a distinct name must not panic.
	cB := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "B"),
	})
	defer cB.Close()

	ctx := context.Background()
	cA.Set(ctx, "ka", []byte("va"))
	_, _ = cA.Get(ctx, "ka") // 1 L1 hit on A
	_, _ = cB.Get(ctx, "kb") // 1 L1 miss on B

	mfs := gather(t, reg)
	hitsA := findMetric(mfs["gokit_cache_hits_total"], map[string]string{"cache": "A", "tier": "L1"})
	hitsB := findMetric(mfs["gokit_cache_hits_total"], map[string]string{"cache": "B", "tier": "L1"})
	if hitsA == nil || hitsB == nil {
		t.Fatalf("missing per-cache hit samples: A=%v B=%v", hitsA, hitsB)
	}
	if got := hitsA.GetCounter().GetValue(); got != 1 {
		t.Errorf("A hits = %v, want 1", got)
	}
	if got := hitsB.GetCounter().GetValue(); got != 0 {
		t.Errorf("B hits = %v, want 0", got)
	}

	missesB := findMetric(mfs["gokit_cache_misses_total"], map[string]string{"cache": "B", "tier": "L1"})
	if missesB == nil {
		t.Fatal("B miss sample missing")
	}
	if got := missesB.GetCounter().GetValue(); got != 1 {
		t.Errorf("B misses = %v, want 1", got)
	}
}

func TestWithMetrics_DuplicateNameSameRegPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	c1 := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "dup"),
	})
	defer c1.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate metric name")
		}
	}()

	// Same name on the same registry: prometheus.MustRegister panics.
	_ = cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(reg, "dup"),
	})
	t.Fatal("expected panic before reaching this line")
}

func TestWithMetrics_NilRegSkips(t *testing.T) {
	// WithMetrics(nil, "x") returns nil — Cache.New treats nil Metrics as
	// disabled (no registration, no panic).
	if mc := cache.WithMetrics(nil, "x"); mc != nil {
		t.Errorf("WithMetrics(nil, x) = %v, want nil", mc)
	}
	if mc := cache.WithMetrics(prometheus.NewRegistry(), ""); mc != nil {
		t.Errorf("WithMetrics(reg, \"\") = %v, want nil", mc)
	}

	// Constructing with a nil *MetricsConfig must not panic and must register
	// nothing on a fresh, separately-passed registerer.
	reg := prometheus.NewRegistry()
	c := cache.New(cache.Config{
		L1MaxItems: 10,
		L1TTL:      time.Minute,
		Metrics:    cache.WithMetrics(nil, "skipped"),
	})
	defer c.Close()
	if mfs, err := reg.Gather(); err != nil || len(mfs) != 0 {
		t.Errorf("expected empty registry, got %d families err=%v", len(mfs), err)
	}
}

func TestWithMetrics_DisabledByDefault(t *testing.T) {
	// Default Config — no Metrics set — must register nothing on a custom
	// registry that the cache never touches.
	reg := prometheus.NewRegistry()
	c := cache.New(cache.Config{L1MaxItems: 10, L1TTL: time.Minute})
	defer c.Close()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(mfs) != 0 {
		t.Errorf("expected 0 metric families on untouched registry, got %d: %v", len(mfs), keys2(mfs))
	}

	// Cache itself must still function normally.
	ctx := context.Background()
	c.Set(ctx, "k", []byte("v"))
	if _, ok := c.Get(ctx, "k"); !ok {
		t.Error("Get returned !ok with metrics disabled — backward compat broken")
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keys2(mfs []*dto.MetricFamily) []string {
	out := make([]string, 0, len(mfs))
	for _, mf := range mfs {
		out = append(out, mf.GetName())
	}
	return out
}
