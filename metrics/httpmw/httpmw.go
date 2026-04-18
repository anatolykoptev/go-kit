// Package httpmw provides RED-method HTTP middleware backed by go-kit/metrics.
//
// RED = Rate (requests_total), Errors (via code label), Duration (request_duration_seconds).
// Optionally tracks response_size_bytes via WithResponseSize.
package httpmw

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anatolykoptev/go-kit/metrics"
)

type config struct {
	responseSize bool
	pathLabel    func(*http.Request) string
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

// Middleware returns an HTTP middleware that records RED metrics via reg.
//
// Metrics written (all use Label encoding with method/path/code labels):
//
//   - <subsystem>_requests_total{method,path,code}   — counter per request
//   - <subsystem>_request_duration_seconds{method,path} — gauge (last observed duration)
//   - <subsystem>_response_size_bytes{method,path}      — gauge, only with WithResponseSize
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
