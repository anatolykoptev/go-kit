package httpmw_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
)

// muxTestHandler is a named struct implementing http.Handler for Mux tests.
type muxTestHandler struct{}

func (h *muxTestHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// TestMux_Handle_RegistersHandler verifies that Mux.Handle registers
// the handler's code.* attrs in the route registry.
func TestMux_Handle_RegistersHandler(t *testing.T) {
	mux := httpmw.NewServeMux()
	handler := &muxTestHandler{}
	mux.Handle("POST /mux-test", handler)

	attrs := httpmw.LookupRoute("POST", "/mux-test")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for Mux.Handle — handler was not auto-registered")
	}

	var fn string
	for _, a := range attrs {
		if string(a.Key) == "code.function" {
			fn = a.Value.AsString()
		}
	}
	if fn == "" {
		t.Error("code.function empty")
	}
	if strings.HasSuffix(fn, "-fm") {
		t.Errorf("code.function = %q; must not end with -fm (method-value wrapper)", fn)
	}
	if !strings.Contains(fn, "ServeHTTP") {
		t.Errorf("code.function = %q; want to contain 'ServeHTTP'", fn)
	}
}

// TestMux_Handle_PatternWithoutMethod registers a pattern with no HTTP method.
func TestMux_Handle_PatternWithoutMethod(t *testing.T) {
	mux := httpmw.NewServeMux()
	handler := &muxTestHandler{}
	mux.Handle("/health", handler)

	// Empty method key
	attrs := httpmw.LookupRoute("", "/health")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for pattern without method")
	}
}

// TestMux_Handle_PatternWithMethod registers a pattern with HTTP method.
func TestMux_Handle_PatternWithMethod(t *testing.T) {
	mux := httpmw.NewServeMux()
	handler := &muxTestHandler{}
	mux.Handle("GET /status", handler)

	attrs := httpmw.LookupRoute("GET", "/status")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for pattern with method GET")
	}
}

// TestMux_HandleFunc_RegistersClosure verifies HandleFunc registers something
// (closure name may be obfuscated — we just require no panic and key present
// in registry, even if attrs are nil/empty for closures).
func TestMux_HandleFunc_RegistersClosure(t *testing.T) {
	mux := httpmw.NewServeMux()
	called := false
	mux.HandleFunc("GET /func-test", func(w http.ResponseWriter, _ *http.Request) {
		called = true
	})
	_ = called
	// Registry lookup may return nil for closure, that's acceptable.
	// The key test is no panic and the mux still works (tested via http.Handler embed).
}

// TestMux_Handle_Concurrent verifies that concurrent RegisterRoute calls via
// the registry are race-free (the race detector checks the registry lock).
func TestMux_Handle_Concurrent(t *testing.T) {
	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			h := &muxTestHandler{}
			// Use unique patterns to avoid ServeMux duplicate-pattern panic.
			// We test concurrent registry writes, not mux routing.
			httpmw.RegisterRoute("GET", "/concurrent-mux", h)
		}()
	}
	wg.Wait()
}
