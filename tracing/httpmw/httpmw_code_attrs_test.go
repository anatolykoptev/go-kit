package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// namedHandlerA is a named function — code.* resolution must succeed.
func namedHandlerA(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// namedHandlerB is a second distinct named function for cross-talk testing.
func namedHandlerB(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func setupRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(sdktrace.NewTracerProvider()) })
	return rec
}

func findSpan(rec *tracetest.SpanRecorder, nameContains string) sdktrace.ReadOnlySpan {
	for _, s := range rec.Ended() {
		if strings.Contains(s.Name(), nameContains) {
			return s
		}
	}
	return nil
}

func attrStr(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func attrInt(s sdktrace.ReadOnlySpan, key string) int64 {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	return -1
}

// TestCodeAttrs_NamedHandler verifies that a span for a named handler carries
// code.namespace, code.function, code.filepath, and code.lineno attributes.
func TestCodeAttrs_NamedHandler(t *testing.T) {
	rec := setupRecorder(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /alpha", namedHandlerA)
	srv := httptest.NewServer(httpmw.Handler("svc", mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/alpha")
	if err != nil {
		t.Fatalf("GET /alpha: %v", err)
	}
	resp.Body.Close()

	span := findSpan(rec, "/alpha")
	if span == nil {
		t.Fatal("no span found for /alpha")
	}

	fn := attrStr(span, "code.function")
	ns := attrStr(span, "code.namespace")
	fp := attrStr(span, "code.filepath")
	ln := attrInt(span, "code.lineno")

	if fn == "" {
		t.Error("code.function missing")
	}
	if !strings.HasSuffix(fn, "namedHandlerA") {
		t.Errorf("code.function = %q; want suffix 'namedHandlerA'", fn)
	}
	if ns == "" {
		t.Error("code.namespace missing")
	}
	if !strings.Contains(ns, "go-kit") {
		t.Errorf("code.namespace = %q; expected to contain 'go-kit'", ns)
	}
	if fp == "" {
		t.Error("code.filepath missing")
	}
	if ln <= 0 {
		t.Errorf("code.lineno = %d; want > 0", ln)
	}
}

// TestCodeAttrs_AnonymousClosure verifies that a span for an anonymous
// closure handler has no code.* attributes, and does not panic.
func TestCodeAttrs_AnonymousClosure(t *testing.T) {
	rec := setupRecorder(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /anon", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(httpmw.Handler("svc", mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/anon")
	if err != nil {
		t.Fatalf("GET /anon: %v", err)
	}
	resp.Body.Close()

	span := findSpan(rec, "/anon")
	if span == nil {
		t.Fatal("no span found for /anon")
	}

	if fn := attrStr(span, "code.function"); fn != "" {
		t.Errorf("code.function = %q; want empty for anonymous closure", fn)
	}
	if ns := attrStr(span, "code.namespace"); ns != "" {
		t.Errorf("code.namespace = %q; want empty for anonymous closure", ns)
	}
}

// TestCodeAttrs_NoXTalk verifies that two routes with distinct named handlers
// each emit their own code.function attribute — no cross-contamination.
func TestCodeAttrs_NoXTalk(t *testing.T) {
	rec := setupRecorder(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /route-a", namedHandlerA)
	mux.HandleFunc("GET /route-b", namedHandlerB)
	srv := httptest.NewServer(httpmw.Handler("svc", mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/route-a")
	if err != nil {
		t.Fatalf("GET /route-a: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/route-b")
	if err != nil {
		t.Fatalf("GET /route-b: %v", err)
	}
	resp.Body.Close()

	spanA := findSpan(rec, "/route-a")
	spanB := findSpan(rec, "/route-b")

	if spanA == nil || spanB == nil {
		t.Fatalf("missing spans: spanA=%v spanB=%v", spanA, spanB)
	}

	fnA := attrStr(spanA, "code.function")
	fnB := attrStr(spanB, "code.function")

	if fnA == fnB {
		t.Errorf("code.function identical on both routes (%q) — cross-talk", fnA)
	}
	if !strings.HasSuffix(fnA, "namedHandlerA") {
		t.Errorf("route-a code.function = %q; want suffix 'namedHandlerA'", fnA)
	}
	if !strings.HasSuffix(fnB, "namedHandlerB") {
		t.Errorf("route-b code.function = %q; want suffix 'namedHandlerB'", fnB)
	}
}
