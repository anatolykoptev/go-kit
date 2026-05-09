package httpmw_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
)

// TestWalkAndRegister_RegistersRoutes verifies WalkAndRegister invokes the
// walk function and registers each (method, pattern, handler) into the registry.
func TestWalkAndRegister_RegistersRoutes(t *testing.T) {
	type routeEntry struct {
		method  string
		pattern string
		handler http.Handler
	}

	routes := []routeEntry{
		{"GET", "/api/v1/things", &muxTestHandler{}},
		{"POST", "/api/v1/things", &muxTestHandler{}},
	}

	walkFn := func(register func(method, pattern string, h http.Handler)) error {
		for _, r := range routes {
			register(r.method, r.pattern, r.handler)
		}
		return nil
	}

	if err := httpmw.WalkAndRegister(walkFn); err != nil {
		t.Fatalf("WalkAndRegister returned error: %v", err)
	}

	for _, r := range routes {
		attrs := httpmw.LookupRoute(r.method, r.pattern)
		if len(attrs) == 0 {
			t.Errorf("route %s %s not registered (no attrs)", r.method, r.pattern)
		}
	}
}

// TestWalkAndRegister_PropagatesError verifies that errors from the walk
// function are returned to the caller.
func TestWalkAndRegister_PropagatesError(t *testing.T) {
	sentinel := errors.New("walk failed")

	walkFn := func(register func(method, pattern string, h http.Handler)) error {
		return sentinel
	}

	err := httpmw.WalkAndRegister(walkFn)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}
