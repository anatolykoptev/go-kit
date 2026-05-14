package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/httpmw"
	"github.com/prometheus/client_golang/prometheus"
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

// ---------------------------------------------------------------------------
// Commit 1: WithDurationHistogram opt-in
// ---------------------------------------------------------------------------

// TestMiddleware_GaugeStillSet verifies existing gauge behaviour is preserved
// when no options are passed (backward compat: gauge consumers must not break).
func TestMiddleware_GaugeStillSet(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_gaustill")
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

// TestMiddleware_HistogramOptIn verifies that WithDurationHistogram also emits
// a histogram alongside the existing gauge (opt-in, not a breaking change).
// The caller supplies a prometheus.Registerer so that tests stay isolated from
// DefaultRegisterer and from each other.
func TestMiddleware_HistogramOptIn(t *testing.T) {
	promReg := prometheus.NewRegistry()
	reg := metrics.NewPrometheusRegistry("t_histopt")
	mw := httpmw.Middleware(reg, "thist", httpmw.WithDurationHistogram(promReg))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/x", nil))

	// histogram must have 2 observations across two requests
	mfs, err := promReg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var totalCount uint64
	var found bool
	for _, mf := range mfs {
		if strings.Contains(mf.GetName(), "request_duration_histogram") {
			found = true
			for _, m := range mf.GetMetric() {
				if m.GetHistogram() == nil {
					t.Fatal("expected histogram metric family, got nil")
				}
				totalCount += m.GetHistogram().GetSampleCount()
			}
		}
	}
	if !found {
		names := make([]string, 0, len(mfs))
		for _, mf := range mfs {
			names = append(names, mf.GetName())
		}
		t.Fatalf("request_duration_histogram metric not found; registered: %v", names)
	}
	if totalCount != 2 {
		t.Fatalf("histogram total _count = %d, want 2", totalCount)
	}

	// gauge must still be set (backward compat)
	key := metrics.Label("thist_request_duration_seconds", "method", "GET", "path", "unknown")
	if v := reg.Gauge(key).Value(); v < 0 {
		t.Fatalf("duration gauge = %f, want >= 0 (gauge must still exist with opt-in)", v)
	}
}
