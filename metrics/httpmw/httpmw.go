// Package httpmw provides RED-method HTTP middleware backed by go-kit/metrics.
//
// RED = Rate (requests_total), Errors (via code label), Duration (request_duration_seconds).
// Optionally tracks response_size_bytes via WithResponseSize.
package httpmw

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type config struct {
	responseSize  bool
	pathLabel     func(*http.Request) string
	histRegisterer prometheus.Registerer
	histBuckets   []float64
}

// Option configures the middleware.
type Option func(*config)

// WithResponseSize enables the optional <subsystem>_response_size_bytes gauge.
// Records the number of bytes written per request.
func WithResponseSize() Option {
	return func(c *config) { c.responseSize = true }
}

// WithPathLabel sets the path-label extractor function.
// Default extractor always returns "unknown" to avoid cardinality explosion
// from raw URL paths. For chi/gorilla-mux pass the route pattern:
//
//	httpmw.WithPathLabel(func(r *http.Request) string {
//	    return chi.RouteContext(r.Context()).RoutePattern()
//	})
func WithPathLabel(fn func(*http.Request) string) Option {
	return func(c *config) { c.pathLabel = fn }
}

// WithDurationHistogram enables an opt-in Prometheus histogram at
// <subsystem>_request_duration_histogram_seconds{method,path} in addition to
// the existing gauge (which is preserved for backward compatibility with
// oxpulse-admin and other consumers already wired against it).
//
// The caller must supply a prometheus.Registerer (e.g. prometheus.DefaultRegisterer
// in production, prometheus.NewRegistry() in tests for isolation). The histogram
// is registered on first request and reused on subsequent calls.
//
// If buckets is empty, prometheus.DefBuckets is used.
//
// Note: the histogram is NOT namespace-prefixed via the metrics.Registry bridge
// because the bridge namespace is unexported. In production, pass
// prometheus.DefaultRegisterer and name accordingly.
func WithDurationHistogram(reg prometheus.Registerer, buckets ...float64) Option {
	return func(c *config) {
		c.histRegisterer = reg
		c.histBuckets = buckets
	}
}

// histogramCache caches registered HistogramVec per subsystem name to avoid
// double-registration across multiple calls to Middleware with the same args.
type histogramCache struct {
	mu    sync.Mutex
	vecs  map[string]*prometheus.HistogramVec // keyed by "<registerer-addr>/<name>"
}

var globalHistCache = &histogramCache{vecs: make(map[string]*prometheus.HistogramVec)}

// getOrRegisterHistVec returns an existing *prometheus.HistogramVec for the given
// (registerer, name, buckets) tuple, or registers a new one. On AlreadyRegisteredError
// with a compatible type, reuses the existing collector (idempotent).
func (hc *histogramCache) getOrRegister(reg prometheus.Registerer, name string, buckets []float64) *prometheus.HistogramVec {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	key := name // per-registerer isolation is handled by the caller using distinct names or registries
	if v, ok := hc.vecs[key]; ok {
		return v
	}
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    "HTTP request duration histogram (opt-in via WithDurationHistogram)",
		Buckets: buckets,
	}, []string{"method", "path"})
	if err := reg.Register(vec); err != nil {
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			if existing, ok := are.ExistingCollector.(*prometheus.HistogramVec); ok {
				hc.vecs[key] = existing
				return existing
			}
		}
		// Shape collision or other error — keep vec off-registry but return it
		// so observations are not silently dropped from the caller's perspective.
		// Subsequent calls will return the same unregistered vec via the cache.
	}
	hc.vecs[key] = vec
	return vec
}

// Middleware returns an HTTP middleware that records RED metrics via reg.
//
// Metrics written (all use Label encoding with method/path/code labels):
//
//   - <subsystem>_requests_total{method,path,code}         — counter per request
//   - <subsystem>_request_duration_seconds{method,path}    — gauge (last observed duration)
//   - <subsystem>_request_duration_histogram_seconds{method,path} — histogram, only with WithDurationHistogram
//   - <subsystem>_response_size_bytes{method,path}         — gauge, only with WithResponseSize
//
// Both prom-backed and in-memory registries are supported. Nil reg is safe (no-op).
func Middleware(reg *metrics.Registry, subsystem string, opts ...Option) func(http.Handler) http.Handler {
	cfg := config{
		pathLabel: func(*http.Request) string { return "unknown" },
	}
	for _, o := range opts {
		o(&cfg)
	}

	reqName := subsystem + "_requests_total"
	durName := subsystem + "_request_duration_seconds"
	sizeName := subsystem + "_response_size_bytes"
	histName := subsystem + "_request_duration_histogram_seconds"

	var histVec *prometheus.HistogramVec
	if cfg.histRegisterer != nil {
		histVec = globalHistCache.getOrRegister(cfg.histRegisterer, histName, cfg.histBuckets)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rw, r)

			path := cfg.pathLabel(r)
			code := strconv.Itoa(rw.status)
			elapsed := time.Since(start).Seconds()

			reg.Incr(metrics.Label(reqName, "method", r.Method, "path", path, "code", code))
			reg.Gauge(metrics.Label(durName, "method", r.Method, "path", path)).Set(elapsed)

			if histVec != nil {
				histVec.WithLabelValues(r.Method, path).Observe(elapsed)
			}

			if cfg.responseSize && rw.written > 0 {
				reg.Gauge(metrics.Label(sizeName, "method", r.Method, "path", path)).Set(float64(rw.written))
			}
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}
