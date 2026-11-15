package mcpmw_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/metrics/mcpmw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
)

// The tool name is request-controlled. A name carrying the Label delimiters
// must be counted on /metrics — not panic the Prometheus bridge (the panic
// ran in the SDK's per-request goroutine, outside any HTTP recover) and not
// be silently dropped either.
//
// RED-on-mutation: revert Label's value sanitizing (metrics/sink.go) — the
// samples are dropped, gokit_metrics_dropped_samples_total moves; revert the
// bridge as well — the call panics.
func TestMiddleware_DelimitersInToolNameAreCounted(t *testing.T) {
	reg := metrics.NewPrometheusRegistry("t_mcp_delim")
	mw := mcpmw.Middleware(reg, "t_mcp_delim")
	ok := makeHandler(func() (*mcp.CallToolResult, error) { return &mcp.CallToolResult{}, nil })

	if _, err := callTool(t, mw, ok, "wp_post"); err != nil {
		t.Fatal(err)
	}
	before := droppedSamples(t)
	for _, name := range []string{"nope,x=y", "a}b{c", `q"`, "x\ny"} {
		if _, err := callTool(t, mw, ok, name); err != nil {
			t.Fatal(err)
		}
		key := metrics.Label("t_mcp_delim_calls_total", "tool", name, "status", "ok")
		if v := reg.Value(key); v != 1 {
			t.Fatalf("calls_total for tool %q = %d, want 1", name, v)
		}
	}
	if d := droppedSamples(t) - before; d != 0 {
		t.Fatalf("%v samples were dropped instead of exported", d)
	}
}

func droppedSamples(t *testing.T) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "gokit_metrics_dropped_samples_total" {
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}
