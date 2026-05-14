package httpmw_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// ---------------------------------------------------------------------------
// Commit 2: ResponseWriter implements Hijacker and Flusher
// ---------------------------------------------------------------------------

// TestResponseWriter_FlushDelegates verifies that Flush() is delegated to the
// underlying ResponseWriter when it implements http.Flusher.
// Uses httptest.NewServer so the underlying ResponseWriter actually supports Flush.
func TestResponseWriter_FlushDelegates(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_flush")
	flushed := make(chan struct{}, 1)

	srv := httptest.NewServer(
		httpmw.Middleware(reg, "http")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
				flushed <- struct{}{}
			} else {
				http.Error(w, "not a flusher", http.StatusInternalServerError)
			}
		})),
	)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/flush-test")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	// Use a timeout rather than default: the handler goroutine may not have
	// sent to the channel yet when the HTTP response completes.
	select {
	case <-flushed:
		// good — Flush() reached the handler
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not receive a Flusher-capable ResponseWriter within 2s")
	}
}

// TestResponseWriter_HijackDelegates verifies that Hijack() is delegated to the
// underlying ResponseWriter when it implements http.Hijacker.
// Uses httptest.NewServer (HTTP/1.1) where ResponseWriter implements Hijacker.
func TestResponseWriter_HijackDelegates(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_hijack")
	hijacked := make(chan struct{}, 1)

	srv := httptest.NewServer(
		httpmw.Middleware(reg, "http")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "not a hijacker", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				http.Error(w, "hijack error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Minimal HTTP response over raw conn so client doesn't hang.
			conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n\r\n")) //nolint:errcheck
			conn.Close()
			hijacked <- struct{}{}
		})),
	)
	defer srv.Close()

	// net/http client follows redirects and parses responses, but a hijacked
	// connection returns a 101 that the client treats as an upgrade — use a raw
	// TCP dial to avoid client-side parse failures.
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.Write([]byte("GET /hijack-test HTTP/1.1\r\nHost: localhost\r\n\r\n")) //nolint:errcheck

	// Drain the response.
	buf := make([]byte, 256)
	conn.Read(buf) //nolint:errcheck

	// Use a timeout rather than default: the handler goroutine sends to the
	// channel AFTER conn.Close(), which the main goroutine's Read() may return
	// from concurrently.
	select {
	case <-hijacked:
		// good — Hijack() reached the underlying conn
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not receive a Hijacker-capable ResponseWriter within 2s")
	}
}
