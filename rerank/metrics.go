package rerank

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// G0 metrics — existing.
	rerankRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_requests_total",
			Help: "Total rerank requests by model and status (ok|error).",
		},
		[]string{"model", "status"},
	)
	rerankDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "rerank_duration_seconds",
			Help: "Rerank request duration by model.",
			Buckets: []float64{
				0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
			},
		},
		[]string{"model"},
	)

	// G1 metrics.

	// rerankRetryAttemptTotal counts each retry attempt (not the initial attempt).
	// Labels: model, attempt (string "1", "2", ...).
	rerankRetryAttemptTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_retry_attempt_total",
			Help: "Total retry attempts after the initial attempt, by model and attempt number.",
		},
		[]string{"model", "attempt"},
	)

	// rerankCircuitState is a gauge tracking the current circuit breaker state
	// (0=closed, 1=open, 2=half-open) per model. Updated on each state change.
	rerankCircuitStateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rerank_circuit_state",
			Help: "Current circuit breaker state: 0=closed, 1=open, 2=half-open.",
		},
		[]string{"model", "state"},
	)

	// rerankCircuitTransitionTotal counts circuit breaker state transitions.
	rerankCircuitTransitionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_circuit_transition_total",
			Help: "Total circuit breaker state transitions by model, from and to state.",
		},
		[]string{"model", "from", "to"},
	)

	// rerankGiveupTotal counts requests that gave up without a successful response.
	// reason: exhausted (retries exhausted), circuit_open, 4xx (non-retryable).
	rerankGiveupTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_giveup_total",
			Help: "Total requests that gave up on retry: reason=exhausted|circuit_open|4xx.",
		},
		[]string{"model", "reason"},
	)

	// rerankFallbackUsedTotal counts successful fallback invocations.
	rerankFallbackUsedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_fallback_used_total",
			Help: "Total successful fallback invocations from primary to secondary model.",
		},
		[]string{"primary", "secondary"},
	)

	// G2-client metrics.

	// rerankScoreDistribution records the distribution of post-pipeline scores.
	// Observed once per doc per call, after Normalize + SourceWeights, before sort.
	rerankScoreDistribution = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rerank_score_distribution",
			Help:    "Distribution of post-pipeline rerank scores by model.",
			Buckets: []float64{-1, -0.5, 0, 0.25, 0.5, 0.75, 1.0, 2.0},
		},
		[]string{"model"},
	)

	// rerankBelowThresholdTotal counts docs dropped by the Threshold filter.
	rerankBelowThresholdTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_below_threshold_total",
			Help: "Total docs dropped by per-call Threshold filter, by model.",
		},
		[]string{"model"},
	)

	// rerankTruncateTotal counts truncation events by model and reason.
	// reason: "tokens" (WithMaxTokensPerDoc) | "chars" (WithMaxCharsPerDoc).
	rerankTruncateTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_truncate_total",
			Help: "Total document truncation events by model and reason (tokens|chars).",
		},
		[]string{"model", "reason"},
	)

	// G3 cascade metrics.

	// rerankCascadeStageInDocs records the input doc count per stage invocation.
	rerankCascadeStageInDocs = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rerank_cascade_stage_in_docs",
			Help:    "Input document count per cascade stage invocation, by label.",
			Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
		},
		[]string{"label"},
	)

	// rerankCascadeStageOutDocs records the output doc count after KeepTopN cut.
	rerankCascadeStageOutDocs = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rerank_cascade_stage_out_docs",
			Help:    "Output document count after KeepTopN cut per cascade stage, by label.",
			Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
		},
		[]string{"label"},
	)

	// rerankCascadeStageOutcomeTotal counts stage completions by outcome.
	// outcome: "ok" | "degraded" | "early_exit"
	rerankCascadeStageOutcomeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_cascade_stage_outcome_total",
			Help: "Total cascade stage completions by label, stage_idx, and outcome (ok|degraded|early_exit).",
		},
		[]string{"label", "stage_idx", "outcome"},
	)

	// rerankCascadeEarlyExitTotal counts cascade early exits by reason.
	rerankCascadeEarlyExitTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rerank_cascade_early_exit_total",
			Help: "Total cascade early exits by label and reason (below_threshold).",
		},
		[]string{"label", "reason"},
	)

	// rerankCascadeTotalDurationSeconds records the total wall time across all cascade stages.
	rerankCascadeTotalDurationSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "rerank_cascade_total_duration_seconds",
			Help:    "Total wall time across all cascade stages (seconds).",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0},
		},
	)
)

// ── G0 helpers ───────────────────────────────────────────────────────────────

func recordStatus(model, status string) {
	rerankRequestsTotal.WithLabelValues(model, status).Inc()
}

func recordDuration(model string, d time.Duration) {
	rerankDurationSeconds.WithLabelValues(model).Observe(d.Seconds())
}

// ── G1 helpers ───────────────────────────────────────────────────────────────

// recordRetryAttempt increments the retry counter for the given model and
// attempt number (1-indexed: 1 = first retry after initial failure).
func recordRetryAttempt(model string, attempt int) {
	rerankRetryAttemptTotal.WithLabelValues(model, itoa(attempt)).Inc()
}

// recordCircuitState updates the circuit state gauge for the given model.
// Only the active state label is set to 1; the others are set to 0.
func recordCircuitState(model string, state CircuitState) {
	states := []CircuitState{CircuitClosed, CircuitOpen, CircuitHalfOpen}
	for _, s := range states {
		v := 0.0
		if s == state {
			v = 1.0
		}
		rerankCircuitStateGauge.WithLabelValues(model, s.String()).Set(v)
	}
}

// recordCircuitTransition increments the transition counter.
func recordCircuitTransition(model string, from, to CircuitState) {
	rerankCircuitTransitionTotal.WithLabelValues(model, from.String(), to.String()).Inc()
}

// recordGiveup increments the giveup counter for the given reason.
func recordGiveup(model, reason string) {
	rerankGiveupTotal.WithLabelValues(model, reason).Inc()
}

// recordFallbackUsed increments the fallback counter.
func recordFallbackUsed(primary, secondary string) {
	rerankFallbackUsedTotal.WithLabelValues(primary, secondary).Inc()
}

// ── G2-client helpers ─────────────────────────────────────────────────────────

// emitScoreDistribution records each score in the post-pipeline distribution
// histogram. Called after Normalize + SourceWeights, before sort.
func emitScoreDistribution(model string, scores []float32) {
	h := rerankScoreDistribution.WithLabelValues(model)
	for _, s := range scores {
		h.Observe(float64(s))
	}
}

// recordBelowThreshold increments the threshold-filter counter by n dropped docs.
func recordBelowThreshold(model string, n int) {
	rerankBelowThresholdTotal.WithLabelValues(model).Add(float64(n))
}

// recordTruncate increments the truncation event counter.
// reason is "tokens" or "chars".
func recordTruncate(model, reason string) {
	rerankTruncateTotal.WithLabelValues(model, reason).Inc()
}

// ── G3 cascade helpers ────────────────────────────────────────────────────────

// recordCascadeStageIn records the number of docs entering a cascade stage.
func recordCascadeStageIn(label string, n int) {
	rerankCascadeStageInDocs.WithLabelValues(label).Observe(float64(n))
}

// recordCascadeStageOut records the number of docs leaving a cascade stage
// after the KeepTopN cut has been applied.
func recordCascadeStageOut(label string, n int) {
	rerankCascadeStageOutDocs.WithLabelValues(label).Observe(float64(n))
}

// recordCascadeStageOutcome increments the stage outcome counter.
// outcome is "ok", "degraded", or "early_exit".
func recordCascadeStageOutcome(label string, stageIdx int, outcome string) {
	rerankCascadeStageOutcomeTotal.WithLabelValues(label, itoa(stageIdx), outcome).Inc()
}

// recordCascadeEarlyExit increments the early-exit counter.
// reason is "below_threshold".
func recordCascadeEarlyExit(label, reason string) {
	rerankCascadeEarlyExitTotal.WithLabelValues(label, reason).Inc()
}

// recordCascadeTotalDuration records the total wall time for a full cascade run.
func recordCascadeTotalDuration(d time.Duration) {
	rerankCascadeTotalDurationSeconds.Observe(d.Seconds())
}

// itoa converts a non-negative integer to its decimal string representation.
// Avoids importing strconv into this file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
