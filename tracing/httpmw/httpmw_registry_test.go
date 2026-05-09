package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// namedHandlerForRegistry is a package-level named func for registry tests.
func namedHandlerForRegistry(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// TestRegisterRoute_NamedFunc verifies that registering a named package-level
// function populates namespace, function, filepath, and lineno in the registry.
func TestRegisterRoute_NamedFunc(t *testing.T) {
	httpmw.RegisterRoute("GET", "/test-named", namedHandlerForRegistry)

	attrs := httpmw.LookupRoute("GET", "/test-named")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for named handler")
	}

	var ns, fn, fp string
	var ln int64 = -1
	for _, a := range attrs {
		switch string(a.Key) {
		case "code.namespace":
			ns = a.Value.AsString()
		case "code.function":
			fn = a.Value.AsString()
		case "code.filepath":
			fp = a.Value.AsString()
		case "code.lineno":
			ln = a.Value.AsInt64()
		}
	}

	if ns == "" {
		t.Error("code.namespace empty")
	}
	if !strings.HasSuffix(fn, "namedHandlerForRegistry") {
		t.Errorf("code.function = %q; want suffix 'namedHandlerForRegistry'", fn)
	}
	if fp == "" {
		t.Error("code.filepath empty")
	}
	if ln <= 0 {
		t.Errorf("code.lineno = %d; want > 0", ln)
	}
}

// TestRegisterRoute_Closure verifies that a closure does not panic and stores
// something (possibly obfuscated).
func TestRegisterRoute_Closure(t *testing.T) {
	closure := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Must not panic.
	httpmw.RegisterRoute("GET", "/test-closure", closure)
	// attrs may be nil/empty for closure; that is acceptable - just no crash.
	_ = httpmw.LookupRoute("GET", "/test-closure")
}

// TestRegisterRoute_Idempotent verifies last call wins when called with same key.
func TestRegisterRoute_Idempotent(t *testing.T) {
	httpmw.RegisterRoute("POST", "/test-idem", namedHandlerForRegistry)
	// Register again - last call wins (same func, same result expected).
	httpmw.RegisterRoute("POST", "/test-idem", namedHandlerForRegistry)
	attrs := httpmw.LookupRoute("POST", "/test-idem")
	if len(attrs) == 0 {
		t.Fatal("no attrs after double registration")
	}
}

// TestRegisterRoute_Concurrent verifies no data race under concurrent
// RegisterRoute + LookupRoute calls (run with -race).
func TestRegisterRoute_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			httpmw.RegisterRoute("GET", "/test-concurrent", namedHandlerForRegistry)
		}()
		go func() {
			defer wg.Done()
			_ = httpmw.LookupRoute("GET", "/test-concurrent")
		}()
	}
	wg.Wait()
}

// TestRegisterRoute_Nil verifies that passing nil as fn is a no-op and does not panic.
func TestRegisterRoute_Nil(t *testing.T) {
	httpmw.RegisterRoute("GET", "/test-nil", nil)
	// No crash; attrs may be nil or empty.
	_ = httpmw.LookupRoute("GET", "/test-nil")
}

// namedHandlerE2E is used for the end-to-end span test.
func namedHandlerE2E(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func setupRecorderForRegistry(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(sdktrace.NewTracerProvider()) })
	return rec
}

func findSpanByPath(rec *tracetest.SpanRecorder, nameContains string) sdktrace.ReadOnlySpan {
	for _, s := range rec.Ended() {
		if strings.Contains(s.Name(), nameContains) {
			return s
		}
	}
	return nil
}

func spanAttrStr(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// TestHandler_E2E_RegisteredRoute verifies that Handler emits code.* attrs on
// a registered named route.
func TestHandler_E2E_RegisteredRoute(t *testing.T) {
	rec := setupRecorderForRegistry(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /e2e-reg", namedHandlerE2E)
	httpmw.RegisterRoute("GET", "/e2e-reg", namedHandlerE2E)

	srv := httptest.NewServer(httpmw.Handler("svc", mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/e2e-reg")
	if err != nil {
		t.Fatalf("GET /e2e-reg: %v", err)
	}
	resp.Body.Close()

	span := findSpanByPath(rec, "/e2e-reg")
	if span == nil {
		t.Fatal("no span for /e2e-reg")
	}

	fn := spanAttrStr(span, "code.function")
	ns := spanAttrStr(span, "code.namespace")
	if fn == "" {
		t.Error("code.function missing on registered route")
	}
	if !strings.HasSuffix(fn, "namedHandlerE2E") {
		t.Errorf("code.function = %q; want suffix namedHandlerE2E", fn)
	}
	if ns == "" {
		t.Error("code.namespace missing on registered route")
	}
}

// TestHandler_E2E_UnregisteredRoute verifies that Handler emits NO code.* attrs
// when route is not in registry.
func TestHandler_E2E_UnregisteredRoute(t *testing.T) {
	rec := setupRecorderForRegistry(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /e2e-unreg", namedHandlerE2E)
	// Intentionally NOT calling RegisterRoute.

	srv := httptest.NewServer(httpmw.Handler("svc", mux))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/e2e-unreg")
	if err != nil {
		t.Fatalf("GET /e2e-unreg: %v", err)
	}
	resp.Body.Close()

	span := findSpanByPath(rec, "/e2e-unreg")
	if span == nil {
		t.Fatal("no span for /e2e-unreg")
	}

	if fn := spanAttrStr(span, "code.function"); fn != "" {
		t.Errorf("code.function = %q; want empty for unregistered route", fn)
	}
}
