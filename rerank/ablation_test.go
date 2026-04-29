package rerank

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAblation_Synthetic503 runs the synthetic 503 ablation harness specified
// in the G1 plan section:
//
//	1000 requests against a server returning 503 30% of the time, 200 70%.
//	With retry+fallback: assert ≥99% return StatusOk or StatusFallback.
//	With retry only:     assert ≥95% return StatusOk.
//	Circuit opens within threshold if 503 rate spikes to 100% for 5 consecutive.
//
// This test is excluded from -short runs to keep CI fast.
func TestAblation_Synthetic503(t *testing.T) {
	if testing.Short() {
		t.Skip("ablation harness skipped in -short mode")
	}

	// ── server that returns 503 with true random probability ────────────────
	// Each HTTP call independently rolls against failRate/100. This avoids
	// deterministic clustering where consecutive retries all fall in a failure
	// burst. The test is parameterised so callers specify any failure rate.
	newFlaky503Server := func(t *testing.T, failRate float64) (*httptest.Server, *atomic.Int64) {
		t.Helper()
		callCount := &atomic.Int64{}
		rng := rand.New(rand.NewPCG(42, 137)) //nolint:gosec
		var mu sync.Mutex                     // guards rng (captured by closure below)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			callCount.Add(1)
			mu.Lock()
			fail := rng.Float64() < failRate
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cohereResponse{
				Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}},
			})
		}))
		t.Cleanup(srv.Close)
		return srv, callCount
	}

	const requests = 1000
	docs := []Doc{{ID: "a", Text: "alpha"}}

	// ── Baseline: no retry, no fallback ────────────────────────────────────
	t.Run("Baseline_NoRetry_NoFallback", func(t *testing.T) {
		srv, _ := newFlaky503Server(t, 0.30)
		c := NewClient(srv.URL,
			WithTimeout(200*time.Millisecond),
			WithRetry(NoRetry),
		)
		var ok, degraded int
		for i := 0; i < requests; i++ {
			res, _ := c.RerankWithResult(context.Background(), "q", docs)
			if res.Status == StatusOk {
				ok++
			} else {
				degraded++
			}
		}
		rate := float64(ok) / float64(requests) * 100
		t.Logf("Baseline | ok=%-4d degraded=%-4d success_rate=%.1f%%", ok, degraded, rate)
		// At 30% failure rate with no retry, we expect ~70% success.
		// No assertion on baseline — this is the measurement point only.
	})

	// ── Retry only ─────────────────────────────────────────────────────────
	t.Run("RetryOnly", func(t *testing.T) {
		srv, _ := newFlaky503Server(t, 0.30)
		c := NewClient(srv.URL,
			WithTimeout(200*time.Millisecond),
			WithRetry(RetryPolicy{
				MaxAttempts:     3,
				BaseBackoff:     0,
				RetryableStatus: []int{503},
			}),
		)
		var ok, degraded int
		for i := 0; i < requests; i++ {
			res, _ := c.RerankWithResult(context.Background(), "q", docs)
			if res.Status == StatusOk {
				ok++
			} else {
				degraded++
			}
		}
		rate := float64(ok) / float64(requests) * 100
		t.Logf("RetryOnly | ok=%-4d degraded=%-4d success_rate=%.1f%%", ok, degraded, rate)
		// With 3 attempts at 30% failure rate: P(all 3 fail) = 0.3^3 = 2.7%.
		// Expected success ≈ 97.3%. Allow 2.3pp slack for statistical variance.
		if rate < 95.0 {
			t.Errorf("retry-only success rate %.1f%% < 95%% required", rate)
		}
	})

	// ── Retry + Fallback ────────────────────────────────────────────────────
	t.Run("RetryAndFallback", func(t *testing.T) {
		primarySrv, _ := newFlaky503Server(t, 0.30)
		// Secondary server always succeeds.
		secondarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cohereResponse{
				Results: []cohereResult{{Index: 0, RelevanceScore: 0.5}},
			})
		}))
		t.Cleanup(secondarySrv.Close)

		secondary := NewClient(secondarySrv.URL,
			WithTimeout(200*time.Millisecond),
			WithRetry(NoRetry),
		)
		primary := NewClient(primarySrv.URL,
			WithTimeout(200*time.Millisecond),
			WithRetry(RetryPolicy{
				MaxAttempts:     3,
				BaseBackoff:     0,
				RetryableStatus: []int{503},
			}),
			WithFallback(secondary),
		)

		var ok, fallback, degraded int
		for i := 0; i < requests; i++ {
			res, _ := primary.RerankWithResult(context.Background(), "q", docs)
			switch res.Status {
			case StatusOk:
				ok++
			case StatusFallback:
				fallback++
			default:
				degraded++
			}
		}
		eventualSuccess := ok + fallback
		rate := float64(eventualSuccess) / float64(requests) * 100
		t.Logf("RetryAndFallback | ok=%-4d fallback=%-4d degraded=%-4d success_rate=%.1f%%",
			ok, fallback, degraded, rate)
		// Secondary always succeeds — so any primary failure should be caught
		// by fallback. Expect near 100%.
		if rate < 99.0 {
			t.Errorf("retry+fallback success rate %.1f%% < 99%% required", rate)
		}
	})

	// ── Circuit opens within threshold at 100%% failure rate ───────────────
	t.Run("CircuitOpensAtThreshold", func(t *testing.T) {
		// Server always returns 503.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)

		c := NewClient(srv.URL,
			WithTimeout(100*time.Millisecond),
			WithRetry(NoRetry),
			WithCircuit(CircuitConfig{
				FailThreshold:  5,
				OpenDuration:   30 * time.Second,
				HalfOpenProbes: 1,
			}),
		)

		// Send requests until circuit opens.
		var circuitOpenedAt int
		for i := 0; i < 20; i++ {
			_, _ = c.RerankWithResult(context.Background(), "q", docs)
			if c.cfg.circuit.State() == CircuitOpen && circuitOpenedAt == 0 {
				circuitOpenedAt = i + 1
			}
		}
		if c.cfg.circuit.State() != CircuitOpen {
			t.Error("circuit should be Open after 5 consecutive 503 failures")
		}
		if circuitOpenedAt > 5 {
			t.Errorf("circuit opened at request %d, expected within 5", circuitOpenedAt)
		}
		t.Logf("CircuitOpensAtThreshold | opened_at_request=%d", circuitOpenedAt)
	})

	t.Logf("%-30s %-15s %-15s", "Config", "Success rate", "Notes")
	t.Logf("Ablation completed — see per-sub-test output above")
}
