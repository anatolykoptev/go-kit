package metrics

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// parseLabeled разбирает имя метрики в синтаксисе kitmetrics.Label():
//
//	"rpc{service=auth,method=login}" → ("rpc", ["service","method"], ["auth","login"])
//	"wp_rest_calls"                   → ("wp_rest_calls", nil, nil).
//
// Невалидный синтаксис (без закрывающей скобки, пустые пары) возвращается как plain.
func parseLabeled(s string) (name string, keys, vals []string) {
	open := strings.IndexByte(s, '{')
	if open < 0 {
		return s, nil, nil
	}
	if !strings.HasSuffix(s, "}") {
		return s, nil, nil
	}
	name = s[:open]
	inner := s[open+1 : len(s)-1]
	if inner == "" {
		return s, nil, nil
	}
	for _, kv := range strings.Split(inner, ",") {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			return s, nil, nil // malformed
		}
		keys = append(keys, kv[:eq])
		vals = append(vals, kv[eq+1:])
	}
	return name, keys, vals
}

// histogramConfig holds per-metric histogram options set via RegisterHistogram.
type histogramConfig struct {
	buckets []float64
}

// HistogramOption is a functional option for RegisterHistogram.
type HistogramOption func(*histogramConfig)

// WithBuckets sets explicit histogram bucket boundaries for the named metric.
// Buckets must be in strictly ascending order (the Prometheus client validates
// this at registration time). Use this for non-time histograms such as bytes,
// queue depth, or request counts — the seconds-shaped default is a poor fit.
//
//	reg.RegisterHistogram("gojob_oversize_bytes",
//	    metrics.WithBuckets([]float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304}))
func WithBuckets(buckets []float64) HistogramOption {
	return func(c *histogramConfig) { c.buckets = buckets }
}

// promBridge отражает операции Registry в prometheus.DefaultRegisterer.
//
// Maps are split per-kind (no-label vs Vec) to avoid type-collision panics
// when the same base name is observed both with and without labels from
// different call sites in the same process. Mixing prometheus.Counter and
// *prometheus.CounterVec under one key caused a forcetypeassert panic in
// production (go-search, ~206 restarts over 52h).
type promBridge struct {
	namespace         string
	countersNoLabel   sync.Map // base name → prometheus.Counter
	countersVec       sync.Map // base name → *prometheus.CounterVec
	gaugesNoLabel     sync.Map // base name → prometheus.Gauge
	gaugesVec         sync.Map // base name → *prometheus.GaugeVec
	histogramsNoLabel sync.Map // base name → prometheus.Histogram
	histogramsVec     sync.Map // base name → *prometheus.HistogramVec
	histogramConfigs  sync.Map // base name → *histogramConfig (pre-registered custom buckets)
}

var nsRe = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// NewPrometheusRegistry создаёт Registry, дополнительно транслирующий операции
// в prometheus.DefaultRegisterer под указанным namespace. namespace должен
// соответствовать [a-zA-Z_:][a-zA-Z0-9_:]* — иначе panic.
func NewPrometheusRegistry(namespace string) *Registry {
	if !nsRe.MatchString(namespace) {
		panic(fmt.Sprintf("metrics: invalid prometheus namespace %q", namespace))
	}
	return &Registry{promBridge: &promBridge{namespace: namespace}}
}

// MetricsHandler возвращает promhttp.Handler() на prometheus.DefaultRegisterer.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// FromEnv возвращает *Registry, prometheus-backed если:
//   - PROM_NAMESPACE задан (используется как namespace), или
//   - METRICS_PROM=1 (используется defaultNamespace).
//
// Иначе возвращает NewRegistry() без prom-bridge.
// Паника если METRICS_PROM=1 и defaultNamespace пуст.
func FromEnv(defaultNamespace string) *Registry {
	if ns := os.Getenv("PROM_NAMESPACE"); ns != "" {
		return NewPrometheusRegistry(ns)
	}
	if os.Getenv("METRICS_PROM") == "1" {
		if defaultNamespace == "" {
			panic("metrics: METRICS_PROM=1 requires non-empty defaultNamespace")
		}
		return NewPrometheusRegistry(defaultNamespace)
	}
	return NewRegistry()
}

// RegisterHistogram pre-configures bucket boundaries for the named histogram
// before the first Observe call. Subsequent Observe calls for name will use
// the configured buckets instead of the seconds-shaped default.
//
// Safe to call on a nil *Registry (no-op). Idempotent: a second call with the
// same name is silently ignored — the first registration wins, matching the
// semantics of the underlying prom collector (buckets are locked at first
// Observe).
//
// name must be a plain metric name (no label syntax); labels are applied at
// Observe time via metrics.Label().
//
// Example:
//
//	reg.RegisterHistogram("gojob_oversize_bytes",
//	    metrics.WithBuckets([]float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304}))
//	reg.Observe("gojob_oversize_bytes", float64(len(payload)))
func (r *Registry) RegisterHistogram(name string, opts ...HistogramOption) {
	if r == nil || r.promBridge == nil {
		return
	}
	cfg := &histogramConfig{
		buckets: prometheus.ExponentialBuckets(0.001, 2, 16), // seconds default
	}
	for _, o := range opts {
		o(cfg)
	}
	// LoadOrStore: first registration wins; subsequent calls for the same name
	// are no-ops. This prevents bucket mutation after the first Observe has
	// already locked the prom histogram in place.
	r.promBridge.histogramConfigs.LoadOrStore(name, cfg)
}
