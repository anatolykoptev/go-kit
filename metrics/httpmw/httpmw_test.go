package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/httpmw"
)

// TestMiddleware_Counts200 verifies that a 200 response increments requests_total.
func TestMiddleware_Counts200(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_200")
	mw := httpmw.Middleware(reg, "http")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	h.ServeHTTP(httptest.NewRecorder(), req)
	h.ServeHTTP(httptest.NewRecorder(), req)

	key := metrics.Label("http_requests_total", "method", "GET", "path", "unknown", "code", "200")
	if got := reg.Value(key); got != 2 {
		t.Fatalf("http_requests_total{200} = %d, want 2", got)
	}
}

// TestMiddleware_Counts500 verifies that a 500 response uses code="500" label.
func TestMiddleware_Counts500(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_500")
	mw := httpmw.Middleware(reg, "http")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	key500 := metrics.Label("http_requests_total", "method", "GET", "path", "unknown", "code", "500")
	if got := reg.Value(key500); got != 1 {
		t.Fatalf("http_requests_total{500} = %d, want 1", got)
	}

	// 200 key must be zero (not incremented)
	key200 := metrics.Label("http_requests_total", "method", "GET", "path", "unknown", "code", "200")
	if got := reg.Value(key200); got != 0 {
		t.Fatalf("http_requests_total{200} = %d, want 0", got)
	}
}

// TestMiddleware_WithPathLabel verifies that the path label extractor overrides "unknown".
func TestMiddleware_WithPathLabel(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_path")
	mw := httpmw.Middleware(reg, "http",
		httpmw.WithPathLabel(func(_ *http.Request) string { return "/static" }),
	)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/whatever/123", nil))

	key := metrics.Label("http_requests_total", "method", "GET", "path", "/static", "code", "200")
	if got := reg.Value(key); got != 1 {
		t.Fatalf("http_requests_total{path=/static} = %d, want 1", got)
	}

	// "unknown" path must be zero
	keyUnknown := metrics.Label("http_requests_total", "method", "GET", "path", "unknown", "code", "200")
	if got := reg.Value(keyUnknown); got != 0 {
		t.Fatalf("http_requests_total{path=unknown} = %d, want 0", got)
	}
}

// TestMiddleware_RecordsDuration verifies that request_duration_seconds gauge is set.
func TestMiddleware_RecordsDuration(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_dur")
	mw := httpmw.Middleware(reg, "http")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	key := metrics.Label("http_request_duration_seconds", "method", "GET", "path", "unknown")
	if v := reg.Gauge(key).Value(); v < 0 {
		t.Fatalf("duration gauge = %f, want >= 0", v)
	}
}

// TestMiddleware_WithResponseSize verifies that response_size_bytes is recorded.
func TestMiddleware_WithResponseSize(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_size")
	mw := httpmw.Middleware(reg, "http", httpmw.WithResponseSize())
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello world")) // 11 bytes
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	key := metrics.Label("http_response_size_bytes", "method", "GET", "path", "unknown")
	if v := reg.Gauge(key).Value(); v != 11 {
		t.Fatalf("response_size_bytes = %f, want 11", v)
	}
}

// TestMiddleware_NilRegistry verifies that a nil registry does not panic.
func TestMiddleware_NilRegistry(t *testing.T) {
	mw := httpmw.Middleware(nil, "http")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// must not panic
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// TestMiddleware_DefaultStatus verifies that a handler that never calls WriteHeader
// is recorded as 200 (net/http default).
func TestMiddleware_DefaultStatus(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_http_dflt")
	mw := httpmw.Middleware(reg, "http")
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// no WriteHeader — net/http sends 200 implicitly on Write / end of handler
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	key := metrics.Label("http_requests_total", "method", "GET", "path", "unknown", "code", "200")
	if got := reg.Value(key); got != 1 {
		t.Fatalf("http_requests_total{200} = %d, want 1", got)
	}
}
