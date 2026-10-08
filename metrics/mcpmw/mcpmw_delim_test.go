package mcpmw_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/mcpmw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The tool name is request-controlled. A name carrying the Label delimiters
// must be counted, not panic the Prometheus bridge — the panic ran in the
// go-sdk per-request goroutine, outside any HTTP recover, and killed the
// process (incident 2026-10-08-gokit-label-comma-mcp-panic).
func TestMiddleware_DelimitersInToolNameDoNotPanic(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_mcp_delim")
	mw := mcpmw.Middleware(reg, "t_mcp_delim")
	ok := makeHandler(func() (*mcp.CallToolResult, error) { return &mcp.CallToolResult{}, nil })

	if _, err := callTool(t, mw, ok, "wp_post"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nope,x=y", "a}b{c", `q"`, "x\ny"} {
		if _, err := callTool(t, mw, ok, name); err != nil {
			t.Fatal(err)
		}
		key := metrics.Label("t_mcp_delim_calls_total", "tool", name, "status", "ok")
		if v := reg.Value(key); v != 1 {
			t.Fatalf("calls_total for tool %q = %d, want 1", name, v)
		}
	}
}
