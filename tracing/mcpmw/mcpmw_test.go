package mcpmw_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anatolykoptev/go-kit/tracing/mcpmw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// makeHandler returns a MethodHandler that delegates tools/call to fn and
// passes other MCP methods through unchanged.
func makeHandler(fn func() (*mcp.CallToolResult, error)) mcp.MethodHandler {
	return func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			return fn()
		}
		return nil, nil
	}
}

func callTool(t *testing.T, mw mcp.Middleware, base mcp.MethodHandler, name string) (mcp.Result, error) {
	t.Helper()
	wrapped := mw(base)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: name}}
	return wrapped(context.Background(), "tools/call", req)
}

// TestMiddleware_NonToolMethodPassesThrough verifies methods other than
// tools/call are not wrapped — those don't carry user-meaningful work.
func TestMiddleware_NonToolMethodPassesThrough(t *testing.T) {
	called := false
	mw := mcpmw.Middleware("test")
	wrapped := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		called = true
		return nil, nil
	})
	if _, err := wrapped(context.Background(), "initialize", nil); err != nil {
		t.Fatalf("non-tool method err: %v", err)
	}
	if !called {
		t.Error("inner handler not called for non-tool method")
	}
}

// TestMiddleware_ToolCallSuccess verifies the result and ctx propagate
// through cleanly when the tool succeeds.
func TestMiddleware_ToolCallSuccess(t *testing.T) {
	res, err := callTool(t, mcpmw.Middleware("svc"),
		makeHandler(func() (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		}),
		"wp_post",
	)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestMiddleware_ToolCallHandlerError verifies handler errors propagate
// without rewrapping (caller controls error semantics).
func TestMiddleware_ToolCallHandlerError(t *testing.T) {
	want := errors.New("boom")
	_, got := callTool(t, mcpmw.Middleware("svc"),
		makeHandler(func() (*mcp.CallToolResult, error) {
			return nil, want
		}),
		"wp_post",
	)
	if !errors.Is(got, want) {
		t.Errorf("err: got %v, want %v", got, want)
	}
}

// TestMiddleware_IsErrorFlagPreserved verifies the IsError flag from the
// tool result is not stripped by the middleware (only marked in span).
func TestMiddleware_IsErrorFlagPreserved(t *testing.T) {
	res, err := callTool(t, mcpmw.Middleware("svc"),
		makeHandler(func() (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{IsError: true}, nil
		}),
		"wp_post",
	)
	if err != nil {
		t.Fatalf("rpc err: %v", err)
	}
	cr, ok := res.(*mcp.CallToolResult)
	if !ok || !cr.IsError {
		t.Error("IsError flag lost in middleware")
	}
}

// TestMiddleware_EmptyToolName verifies a request without parameters yields
// an empty tool name (span name falls back to "mcp.tools.call ").
func TestMiddleware_EmptyToolName(t *testing.T) {
	mw := mcpmw.Middleware("svc")
	wrapped := mw(makeHandler(func() (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}))
	// Pass empty name — middleware must not panic.
	if _, err := wrapped(context.Background(), "tools/call",
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: ""}}); err != nil {
		t.Fatalf("err: %v", err)
	}
}

// callToolWithClientParams exercises the *mcp.CallToolParams branch of
// extractToolName (defensive guard for future SDK changes or unexpected wrapping).
// In the current SDK (v1.5.0) the server MethodHandler always receives
// *CallToolParamsRaw; this test passes a synthetic ClientRequest to confirm
// the switch arm compiles and does not panic.
func TestExtractToolName_TypedParams(t *testing.T) {
	mw := mcpmw.Middleware("svc")
	wrapped := mw(makeHandler(func() (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}))

	// Construct a request carrying *CallToolParams (client-side type).
	// mcp.ClientRequest is the generic counterpart of ServerRequest.
	type clientCallReq = mcp.ClientRequest[*mcp.CallToolParams]
	req := &clientCallReq{Params: &mcp.CallToolParams{Name: "my_tool"}}

	res, err := wrapped(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	// We can't easily inspect the span name in a unit test without a real
	// TracerProvider; we rely on no-panic + correct result propagation.
}
