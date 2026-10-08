package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// A label value carrying the Label delimiters must stay ONE label value.
// Before the fix Label("m", "tool", "nope,x=y") parsed back as two labels,
// and the Prometheus bridge panicked on the extra value inside the caller's
// goroutine — one MCP tools/call with a comma in the tool name killed the
// process (incident 2026-10-08-gokit-label-comma-mcp-panic).
//
// RED-on-mutation: in Label (metrics/sink.go) write kvs[i+1] instead of
// labelSanitizer.Replace(kvs[i+1]); parseLabeled then returns two keys.
func TestLabel_DelimitersInValueStayOneLabel(t *testing.T) {
	for _, v := range []string{"nope,x=y", "a{b}c", `q"uote`, "line\nbreak", "cr\rlf", "a=b", "x,y,z"} {
		name, keys, vals := parseLabeled(Label("m", "tool", v))
		if name != "m" || len(keys) != 1 || keys[0] != "tool" || len(vals) != 1 {
			t.Fatalf("Label(m, tool, %q) parsed as name=%q keys=%q vals=%q, want one tool label", v, name, keys, vals)
		}
	}
	if got := Label("m", "tool", "wp_post", "status", "ok"); got != "m{tool=wp_post,status=ok}" {
		t.Fatalf("plain labels must be unchanged, got %q", got)
	}
}

// The bridge must never panic on a label set the registered vec rejects,
// whatever produced the key (a hand-built string bypasses Label entirely).
//
// RED-on-mutation: in observeCounter (metrics/prom_bridge.go) go back to
// b.counterVec(base, keys).WithLabelValues(vals...).Add(delta); the second
// Incr panics with "inconsistent label cardinality".
func TestPromBridge_LabelCountMismatchDoesNotPanic(t *testing.T) {
	reg := NewPrometheusRegistry("labelmismatch")
	reg.Incr("calls_total{tool=a}")
	reg.Gauge("dur{tool=a}").Set(1)
	reg.ObserveSeconds("lat{tool=a}", time.Second)

	start := collisionCount()
	reg.Incr("calls_total{tool=a,x=y}")
	reg.Gauge("dur{tool=a,x=y}").Set(2)
	reg.ObserveSeconds("lat{tool=a,x=y}", time.Second)
	if got := collisionCount() - start; got != 3 {
		t.Fatalf("dropped samples must be counted, collision delta = %v, want 3", got)
	}
	vec := reg.promBridge.counterVec("calls_total", []string{"tool"})
	if got := testutil.ToFloat64(vec.WithLabelValues("a")); got != 1 {
		t.Fatalf("registered series = %v, want 1", got)
	}
}
