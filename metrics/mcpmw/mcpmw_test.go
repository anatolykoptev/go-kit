package mcpmw_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/mcpmw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// makeHandler returns a mcp.MethodHandler that delegates tools/call to fn
// and passes other methods through unchanged.
func makeHandler(fn func() (*mcp.CallToolResult, error)) mcp.MethodHandler {
	return func(_ context.Context, method string, _ mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			return fn()
		}
		return nil, nil
	}
}

// callTool invokes a middleware chain for "tools/call" with the given tool name.
func callTool(
	t *testing.T,
	mw mcp.Middleware,
	base mcp.MethodHandler,
	toolName string,
) (mcp.Result, error) {
	t.Helper()
	wrapped := mw(base)
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: toolName},
	}
	return wrapped(context.Background(), "tools/call", req)
}

// TestMiddleware_SuccessPath verifies that a successful tool call increments
// <subsystem>_calls_total with status="ok" and records duration.
func TestMiddleware_SuccessPath(t *testing.T) {
	reg := metrics.NewRegistry()
	mw := mcpmw.Middleware(reg, "t_mcp_ok")

	_, err := callTool(t, mw, makeHandler(func() (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}), "wp_post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := metrics.Label("t_mcp_ok_calls_total", "tool", "wp_post", "status", "ok")
	if v := reg.Value(key); v != 1 {
		t.Errorf("calls_total{status=ok} = %d, want 1", v)
	}
	errKey := metrics.Label("t_mcp_ok_calls_total", "tool", "wp_post", "status", "error")
	if v := reg.Value(errKey); v != 0 {
		t.Errorf("calls_total{status=error} = %d, want 0", v)
	}
}

// TestMiddleware_ErrorPath verifies that a tool call returning an error
// increments <subsystem>_calls_total with status="error".
func TestMiddleware_ErrorPath(t *testing.T) {
	reg := metrics.NewRegistry()
	mw := mcpmw.Middleware(reg, "t_mcp_err")
	boom := errors.New("boom")

	_, _ = callTool(t, mw, makeHandler(func() (*mcp.CallToolResult, error) {
		return nil, boom
	}), "wp_seo")

	key := metrics.Label("t_mcp_err_calls_total", "tool", "wp_seo", "status", "error")
	if v := reg.Value(key); v != 1 {
		t.Errorf("calls_total{status=error} = %d, want 1", v)
	}
	okKey := metrics.Label("t_mcp_err_calls_total", "tool", "wp_seo", "status", "ok")
	if v := reg.Value(okKey); v != 0 {
		t.Errorf("calls_total{status=ok} = %d, want 0", v)
	}
}

// TestMiddleware_IsErrorFlag verifies that a result with IsError=true counts
// as status="error" even when the Go error is nil.
func TestMiddleware_IsErrorFlag(t *testing.T) {
	reg := metrics.NewRegistry()
	mw := mcpmw.Middleware(reg, "t_mcp_isErr")

	_, _ = callTool(t, mw, makeHandler(func() (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	}), "wp_cli")

	key := metrics.Label("t_mcp_isErr_calls_total", "tool", "wp_cli", "status", "error")
	if v := reg.Value(key); v != 1 {
		t.Errorf("calls_total{IsError} = %d, want 1", v)
	}
}

// TestMiddleware_NonToolMethod verifies that non-tool methods are passed through
// without recording any metrics.
func TestMiddleware_NonToolMethod(t *testing.T) {
	reg := metrics.NewRegistry()
	mw := mcpmw.Middleware(reg, "t_mcp_pass")

	wrapped := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, nil
	})
	_, err := wrapped(context.Background(), "tools/list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No counter should have been incremented.
	if v := reg.Value("t_mcp_pass_calls_total"); v != 0 {
		t.Errorf("non-tool method incremented counter = %d", v)
	}
}

// TestMiddleware_NilRegistry verifies nil-safety: middleware still calls the
// handler but does not panic.
func TestMiddleware_NilRegistry(t *testing.T) {
	mw := mcpmw.Middleware(nil, "t_mcp_nil")
	called := false
	base := makeHandler(func() (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	})
	_, err := callTool(t, mw, base, "any_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler not called on nil registry")
	}
}

// TestMiddleware_DurationRecorded verifies that a non-zero duration gauge is
// set after a tool call with a small artificial delay.
func TestMiddleware_DurationRecorded(t *testing.T) {
	reg := metrics.NewRegistry()
	mw := mcpmw.Middleware(reg, "t_mcp_dur")

	_, _ = callTool(t, mw, makeHandler(func() (*mcp.CallToolResult, error) {
		time.Sleep(2 * time.Millisecond)
		return &mcp.CallToolResult{}, nil
	}), "slow_tool")

	g := reg.Gauge(metrics.Label("t_mcp_dur_duration_seconds", "tool", "slow_tool"))
	if g.Value() <= 0 {
		t.Errorf("duration gauge = %v, want > 0", g.Value())
	}
}
