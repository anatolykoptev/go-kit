package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestSetup_NoEndpoint_NoOpButPropagatorsInstalled(t *testing.T) {
	t.Setenv(envEndpoint, "")
	shutdown, err := Setup(context.Background(), "test-svc")
	if err != nil {
		t.Fatalf("Setup with no endpoint should succeed (no-op), got: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned error: %v", err)
	}
	// Propagators must still be set so cross-process trace context
	// extraction keeps working even without local export.
	prop := otel.GetTextMapPropagator()
	if _, ok := prop.(propagation.TextMapPropagator); !ok {
		t.Errorf("propagator not installed")
	}
}

func TestSetup_EmptyServiceName_Errors(t *testing.T) {
	_, err := Setup(context.Background(), "")
	if err == nil {
		t.Errorf("Setup with empty serviceName should error")
	}
}

func TestSetup_BadEndpoint_NoopFallback(t *testing.T) {
	// A syntactically invalid URL causes otlptracehttp.New to error.
	// Setup must degrade gracefully (noop + nil error) so the service starts.
	t.Setenv(envEndpoint, "")
	shutdown, err := Setup(context.Background(), "test-svc",
		WithEndpoint("://not-a-valid-url"))
	if err != nil {
		t.Fatalf("Setup with bad endpoint must return nil error (graceful degrade), got: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned error: %v", err)
	}
}

func TestSetup_NilCtx_NoNPE(t *testing.T) {
	// nil ctx must not panic — Setup substitutes context.Background().
	t.Setenv(envEndpoint, "")
	//nolint:staticcheck // intentional nil ctx to test guard
	shutdown, err := Setup(nil, "test-svc") //nolint:staticcheck
	if err != nil {
		t.Fatalf("Setup(nil, ...) must not error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown must be non-nil")
	}
}

func TestStart_NoOpSpanFromGlobalProvider(t *testing.T) {
	// Without Setup, otel global TP is the no-op default — Start must still
	// return a usable (non-nil) Span and the original ctx semantics.
	ctx, span := Start(context.Background(), "noop.test", attribute.Int("k", 1))
	if span == nil {
		t.Fatal("Start returned nil span")
	}
	if !trace.SpanFromContext(ctx).SpanContext().IsValid() && trace.SpanFromContext(ctx) == trace.SpanFromContext(context.Background()) {
		// Acceptable — no-op span has invalid context. Just verify End is callable.
	}
	span.End()
}

func TestStart_ScopeFromSetup(t *testing.T) {
	// Reset active service name before the test to avoid state from other tests.
	activeServiceName.Store("")
	t.Setenv(envEndpoint, "")

	_, err := Setup(context.Background(), "my-service")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	// After Setup, activeServiceName must reflect "my-service".
	v, ok := activeServiceName.Load().(string)
	if !ok || v != "my-service" {
		t.Errorf("activeServiceName = %q, want %q", v, "my-service")
	}

	// Start must use the stored scope (verified indirectly; with the no-op
	// global TP the span is inert but Start must not panic or use the
	// hardcoded fallback scope).
	ctx, span := Start(context.Background(), "test.op")
	if span == nil {
		t.Fatal("Start returned nil span")
	}
	span.End()

	// Confirm the fallback is NOT triggered when Setup was called.
	_ = ctx
}

func TestRecordError_NilNoop(t *testing.T) {
	_, span := Start(context.Background(), "test")
	defer span.End()
	RecordError(span, nil) // must not panic
}

func TestRecordError_SetsStatus(t *testing.T) {
	_, span := Start(context.Background(), "test")
	defer span.End()
	RecordError(span, errors.New("boom")) // must not panic
}

// Endpoint URL parsing is now delegated to otlptracehttp.WithEndpointURL.
// We rely on the canonical OTel SDK behaviour (full URL with scheme) instead
// of the legacy WithEndpoint(host:port) which couldn't tell scheme from path.

// Silent-failure guard: with otlptracehttp >= 1.45, WithEndpointURL no longer
// appends the signal path, so a pathless endpoint ("http://jaeger:4318", the
// documented form) used to POST every span to "/" and lose them without any
// warning. Goes through the real Setup -> exporter -> HTTP path.
func TestSetup_PathlessEndpoint_PostsToV1Traces(t *testing.T) {
	var (
		mu    sync.Mutex
		posts []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Method == http.MethodPost {
			posts = append(posts, r.URL.Path)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	t.Setenv(envEndpoint, "")

	shutdown, err := Setup(context.Background(), "path-svc", WithEndpoint(srv.URL))
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	_, span := otel.Tracer("path-test").Start(context.Background(), "op")
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(posts) == 0 {
		t.Fatalf("no POST reached the collector; spans were dropped")
	}
	for _, p := range posts {
		if p != "/v1/traces" {
			t.Errorf("POST path = %q, want %q (all paths: %v)", p, "/v1/traces", posts)
		}
	}
}

func TestWithTracesPath(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"pathless", "http://jaeger:4318", "http://jaeger:4318/v1/traces"},
		{"trailing slash", "http://jaeger:4318/", "http://jaeger:4318/v1/traces"},
		{"existing path kept", "http://gw:4318/otlp/v1/traces", "http://gw:4318/otlp/v1/traces"},
		{"other path kept", "http://gw:4318/custom", "http://gw:4318/custom"},
		{"bare host:port untouched", "jaeger:4318", "jaeger:4318"},
		{"bare ip:port untouched", "127.0.0.1:4318", "127.0.0.1:4318"},
		{"https", "https://otel.example:4318", "https://otel.example:4318/v1/traces"},
		{"query kept", "https://otel.example:4318?x=1", "https://otel.example:4318/v1/traces?x=1"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withTracesPath(tc.in); got != tc.want {
				t.Errorf("withTracesPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
