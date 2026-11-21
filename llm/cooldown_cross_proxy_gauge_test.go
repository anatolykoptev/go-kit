package llm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/llm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestCooldownGauge_StaysActiveWhileAnotherProxyStillCools is the regression
// guard for issue #313: the active gauge is labelled by model, while the
// dead-model cooldown is keyed by (URL, model). When two proxies route the same
// model and only one of them recovers, the model is STILL cooled — but the
// recovery edge fired by the recovered proxy's 200 cleared
// llm_model_cooldown_active{model} to 0, claiming the model was healthy again.
//
// Routing was never affected (endpointCooling still consults both keys); only
// the metric lied. Post-fix the recovery edge waits for the last live window of
// that model.
func TestCooldownGauge_StaysActiveWhileAnotherProxyStillCools(t *testing.T) {
	cm := llm.NewChainMetrics(prometheus.NewRegistry())

	var aHealthy atomic.Bool
	proxyA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !aHealthy.Load() {
			statusBodyHandler(http.StatusBadGateway, unknownProvider502)(w, r)
			return
		}
		okChatHandler(t, "from-proxy-a")(w, r)
	}))
	defer proxyA.Close()
	proxyB := httptest.NewServer(statusBodyHandler(http.StatusBadGateway, unknownProvider502))
	defer proxyB.Close()

	// Count the model-level cooldown ENTRY edges: two endpoint keys cooling for the
	// same model is one model-level cooling period, so the observer must fire once.
	var entries atomic.Int32
	observer := cm.CooldownObserver()
	countingObserver := func(model string, cooling bool, d time.Duration) {
		if cooling {
			entries.Add(1)
		}
		observer(model, cooling, d)
	}

	c := llm.NewClient("", "", "",
		llm.WithEndpoints([]llm.Endpoint{
			{URL: proxyA.URL, Key: "k", Model: "m"},
			{URL: proxyB.URL, Key: "k", Model: "m"},
		}),
		llm.WithMaxRetries(1),
		llm.WithModelCooldown(llm.CooldownConfig{FailThreshold: 1}),
		llm.WithModelCooldownObserver(countingObserver),
	)

	// Call 1: both proxies answer 502 "unknown provider for model m". The
	// dead-model key is (URL, model), so BOTH keys cool — one model, two windows.
	if _, err := c.Complete(context.Background(), "", "hi"); err == nil {
		t.Fatal("call 1: expected the upstream 502 error")
	}
	gauge := cm.CooldownActive().WithLabelValues("m")
	waitGauge(t, gauge, 1, "both proxies cooled")

	// Proxy A recovers. Call 2 finds every key cooled, so it never fail-closes:
	// it force-attempts the primary (A) and stops on its 200, which clears A's
	// key. B's key is never touched — no request reaches B again.
	aHealthy.Store(true)
	out, err := c.Complete(context.Background(), "", "hi")
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if out != "from-proxy-a" {
		t.Fatalf("call 2 output = %q, want from-proxy-a", out)
	}

	// THE assertion: proxy B is still cooled, so the model-level gauge must stay
	// at 1. Pre-fix, A's 200 fired the recovery edge and drove it to 0.
	settleGauge(t, gauge, 1)

	// And the model got ONE entry edge, not one per cooled endpoint: the second
	// key's entry is already covered by the live gauge and would only add a
	// goroutine that can be scheduled after the model's recovery edge. Checked
	// here, after both call-1 observers have had the whole call-2 round trip to
	// land, so this cannot read a half-delivered count.
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := entries.Load(); got != 1 {
			t.Fatalf("model-level entry edges = %d after two endpoints cooled, want 1", got)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// settleGauge asserts a gauge never leaves want over a short window. The
// cooldown observers are dispatched on their own goroutines (fireCooldownObserver),
// so an erroneous recovery edge cannot be ruled out by a single read: 100ms is
// far longer than the dispatch being waited out. A correct gauge simply stays
// put, so nothing here can flake in the pass direction.
func settleGauge(t *testing.T, g prometheus.Gauge, want float64) {
	t.Helper()
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := testutil.ToFloat64(g); got != want {
			t.Fatalf("gauge moved to %v while a proxy was still cooled, want %v", got, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
