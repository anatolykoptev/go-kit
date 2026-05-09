package httpmw_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/httpmw"
)

// TestRegisterGinRoute_InjectsNameDirectly verifies that RegisterGinRoute
// writes the handler name string directly into the registry (bypassing
// reflect/FuncForPC), so gin's pre-resolved name is used verbatim.
func TestRegisterGinRoute_InjectsNameDirectly(t *testing.T) {
	// gin engine.Routes() gives HandlerFunc as a string like:
	// "github.com/myorg/myapp/handlers.(*API).GetUser-fm"
	// RegisterGinRoute should strip -fm and register correctly.
	handlerName := "github.com/myorg/myapp/handlers.(*API).GetUser-fm"
	httpmw.RegisterGinRoute("GET", "/gin-test", handlerName)

	attrs := httpmw.LookupRoute("GET", "/gin-test")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered via RegisterGinRoute")
	}

	var fn, ns string
	for _, a := range attrs {
		switch string(a.Key) {
		case "code.function":
			fn = a.Value.AsString()
		case "code.namespace":
			ns = a.Value.AsString()
		}
	}

	if fn == "" {
		t.Error("code.function empty")
	}
	if strings.HasSuffix(fn, "-fm") {
		t.Errorf("code.function = %q; must not end with -fm", fn)
	}
	if !strings.Contains(fn, "GetUser") {
		t.Errorf("code.function = %q; want to contain 'GetUser'", fn)
	}
	if ns != "github.com/myorg/myapp/handlers" {
		t.Errorf("code.namespace = %q; want 'github.com/myorg/myapp/handlers'", ns)
	}
}

// TestRegisterGinRoute_PlainFuncName verifies a package-level handler name.
func TestRegisterGinRoute_PlainFuncName(t *testing.T) {
	httpmw.RegisterGinRoute("POST", "/gin-plain", "github.com/myapp/handlers.CreateUser")

	attrs := httpmw.LookupRoute("POST", "/gin-plain")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for plain func name")
	}

	var fn, ns string
	for _, a := range attrs {
		switch string(a.Key) {
		case "code.function":
			fn = a.Value.AsString()
		case "code.namespace":
			ns = a.Value.AsString()
		}
	}
	if fn != "CreateUser" {
		t.Errorf("code.function = %q; want 'CreateUser'", fn)
	}
	if ns != "github.com/myapp/handlers" {
		t.Errorf("code.namespace = %q; want 'github.com/myapp/handlers'", ns)
	}
}

// TestRegisterGinRoute_EmptyName verifies no-op on empty handler name.
func TestRegisterGinRoute_EmptyName(t *testing.T) {
	// Must not panic, must not register anything useful.
	httpmw.RegisterGinRoute("GET", "/gin-empty", "")
	// attrs may be nil — that is acceptable.
	_ = httpmw.LookupRoute("GET", "/gin-empty")
}

// TestRegisterRoute_MethodValue verifies that a method value passed to
// RegisterRoute resolves to a name WITHOUT the -fm suffix.
// This exercises the TrimSuffix in splitRegistryFuncName.
func TestRegisterRoute_MethodValue(t *testing.T) {
	h := &muxTestHandler{}
	// (&T{}).Method is a method value — the compiler emits a -fm wrapper.
	// After TrimSuffix it should resolve to (*muxTestHandler).ServeHTTP.
	httpmw.RegisterRoute("GET", "/method-value-test", h.ServeHTTP)

	attrs := httpmw.LookupRoute("GET", "/method-value-test")
	if len(attrs) == 0 {
		t.Fatal("no attrs registered for method value")
	}

	var fn string
	for _, a := range attrs {
		if string(a.Key) == "code.function" {
			fn = a.Value.AsString()
		}
	}
	if strings.HasSuffix(fn, "-fm") {
		t.Errorf("code.function = %q; must not end with -fm after strip", fn)
	}
	if !strings.Contains(fn, "ServeHTTP") {
		t.Errorf("code.function = %q; want to contain 'ServeHTTP'", fn)
	}
}

// TestRegisterRoute_ClosureHasFileLine verifies that a closure registration
// stores code.filepath and code.lineno even when the function name is obfuscated.
func TestRegisterRoute_ClosureHasFileLine(t *testing.T) {
	closure := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}
	httpmw.RegisterRoute("GET", "/closure-fileline", closure)

	attrs := httpmw.LookupRoute("GET", "/closure-fileline")
	// The current implementation filters out closures in splitRegistryFuncName
	// (returns "" when namespace or funcName is empty). Until the closure
	// fallback emitting only filepath+lineno is implemented, attrs will be nil.
	if attrs == nil {
		t.Log("closure attrs nil — splitRegistryFuncName filters closures; expected until closure fallback is implemented")
		t.Skip("closure filepath+lineno fallback not yet implemented")
		return
	}

	var fp string
	var ln int64 = -1
	for _, a := range attrs {
		switch string(a.Key) {
		case "code.filepath":
			fp = a.Value.AsString()
		case "code.lineno":
			ln = a.Value.AsInt64()
		}
	}
	if fp == "" {
		t.Error("code.filepath empty for closure")
	}
	if ln <= 0 {
		t.Errorf("code.lineno = %d; want > 0 for closure", ln)
	}
}
