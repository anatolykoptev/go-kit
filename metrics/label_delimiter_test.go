package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// A request-controlled label value (an MCP tool name, a route) must stay ONE
// label value. An unescaped ',' made Label("m", "tool", "nope,x=y") parse
// back as two labels, and the Prometheus bridge then panicked on the label
// count mismatch in the observing goroutine.
//
// RED-on-mutation: in Label (metrics/sink.go) write kvs[i+1] instead of
// labelValueSanitizer.Replace(kvs[i+1]); parseLabeled then returns two keys.
func TestLabel_DelimitersInValueStayOneLabel(t *testing.T) {
	for _, v := range []string{"nope,x=y", "x,y,z", "line\nbreak", "cr\rlf"} {
		name, keys, vals := parseLabeled(Label("m", "tool", v))
		if name != "m" || len(keys) != 1 || keys[0] != "tool" || len(vals) != 1 {
			t.Fatalf("Label(m, tool, %q) parsed as name=%q keys=%q vals=%q, want one tool label", v, name, keys, vals)
		}
	}
	// Only what breaks parsing is replaced: these values keep their series names.
	for _, v := range []string{"a=b", "GET /items/{id}", `q"x`, "wp_post"} {
		_, keys, vals := parseLabeled(Label("m", "path", v))
		if len(keys) != 1 || vals[0] != v {
			t.Fatalf("Label(m, path, %q) must round-trip unchanged, got keys=%q vals=%q", v, keys, vals)
		}
	}
	if _, keys, _ := parseLabeled(Label("m", "a=b{c}", "v")); len(keys) != 1 || keys[0] != "a_b_c_" {
		t.Fatalf("a key's delimiters must be replaced, got keys=%q", keys)
	}
}

// The bridge must never panic on a label set the registered vec rejects,
// whatever built the key (a hand-written key bypasses Label), and must count
// what it drops.
//
// RED-on-mutation: in observeCounter (metrics/prom_bridge.go) go back to
// b.counterVec(base, keys).WithLabelValues(vals...).Add(delta); the second
// Incr panics with "inconsistent label cardinality".
func TestPromBridge_RejectedLabelSetDoesNotPanic(t *testing.T) {
	reg := NewPrometheusRegistry("labelmismatch")
	reg.Incr("calls_total{tool=a}")
	reg.Gauge("dur{tool=a}").Set(1)
	reg.ObserveSeconds("lat{tool=a}", time.Second)

	startDrop, startCol := droppedCount(), collisionCount()
	reg.Incr("calls_total{tool=a,x=y}")
	reg.Gauge("dur{tool=a,x=y}").Set(2)
	reg.ObserveSeconds("lat{tool=a,x=y}", time.Second)
	reg.Incr(Label("calls_total", "tool", "bad\xff")) // invalid UTF-8 value
	if got := droppedCount() - startDrop; got != 4 {
		t.Fatalf("dropped samples must be counted, delta = %v, want 4", got)
	}
	if got := collisionCount() - startCol; got != 0 {
		t.Fatalf("a rejected label set is not a shape collision, collision delta = %v", got)
	}
	vec := reg.promBridge.counterVec("calls_total", []string{"tool"})
	if got := testutil.ToFloat64(vec.WithLabelValues("a")); got != 1 {
		t.Fatalf("registered series = %v, want 1", got)
	}
}

func droppedCount() float64 {
	if droppedSampleCounter == nil {
		return 0
	}
	return testutil.ToFloat64(droppedSampleCounter)
}
