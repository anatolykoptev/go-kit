package rerank

import (
	"context"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// getCounterValue is a helper that reads the current value of a Prometheus Counter.
func getCounterValue(t *testing.T, c interface{ Write(*dto.Metric) error }) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter.Write: %v", err)
	}
	if m.Counter == nil {
		return 0
	}
	return m.Counter.GetValue()
}

func TestMathReranker_PureCosineRanking_KnownVectors(t *testing.T) {
	qv := []float32{1, 0, 0}
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{0, 1, 0}},  // orthogonal → sim 0
		{ID: "b", EmbedVector: []float32{1, 0, 0}},  // parallel → sim 1
		{ID: "c", EmbedVector: []float32{-1, 0, 0}}, // antiparallel → sim -1
	}
	m := MathReranker{QueryVector: qv, Lambda: 0}
	res, err := m.RerankWithResult(context.Background(), "", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if res.Model != "math" {
		t.Errorf("model: got %q want \"math\"", res.Model)
	}
	if len(res.Scored) != 3 {
		t.Fatalf("len: got %d want 3", len(res.Scored))
	}
	// Expected order: b(1), a(0), c(-1)
	wantIDs := []string{"b", "a", "c"}
	for i, id := range wantIDs {
		if res.Scored[i].ID != id {
			t.Errorf("pos %d: got %q want %q", i, res.Scored[i].ID, id)
		}
	}
}

func TestMathReranker_EmptyQueryVector_StatusSkipped(t *testing.T) {
	m := MathReranker{QueryVector: nil, Lambda: 0}
	docs := []Doc{{ID: "a", EmbedVector: []float32{1, 0}}}
	res, err := m.RerankWithResult(context.Background(), "", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("status: got %v want Skipped", res.Status)
	}
	if len(res.Scored) != 1 || res.Scored[0].ID != "a" {
		t.Errorf("passthrough not preserved: %+v", res.Scored)
	}
}

func TestMathReranker_MissingDocVector_ScoreZero(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{1, 0}}, // sim 1
		{ID: "b"},                               // no EmbedVector → score 0
	}
	res, _ := m.RerankWithResult(context.Background(), "", docs)
	if res.Status != StatusOk {
		t.Fatalf("status: %v", res.Status)
	}
	// b should have score 0
	for _, s := range res.Scored {
		if s.ID == "b" && s.Score != 0 {
			t.Errorf("doc b: score=%v want 0", s.Score)
		}
	}
	// a should be ranked first
	if res.Scored[0].ID != "a" {
		t.Errorf("top result: got %q want a", res.Scored[0].ID)
	}
}

func TestMathReranker_AllDocsEmpty_PassthroughOrder(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "a"}, // no EmbedVector
		{ID: "b"}, // no EmbedVector
	}
	res, _ := m.RerankWithResult(context.Background(), "", docs)
	if res.Status != StatusOk {
		t.Fatalf("status: %v", res.Status)
	}
	if len(res.Scored) != 2 {
		t.Fatalf("len: got %d want 2", len(res.Scored))
	}
	// All scores are 0 → stable sort preserves original order.
	if res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("stable order not preserved: %q %q", res.Scored[0].ID, res.Scored[1].ID)
	}
}

func TestMathReranker_Lambda0_NoMMR_PureRelevance(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "low", EmbedVector: []float32{0, 1}},  // orthogonal → sim 0
		{ID: "high", EmbedVector: []float32{1, 0}}, // parallel → sim 1
	}
	res, _ := m.RerankWithResult(context.Background(), "", docs)
	if res.Scored[0].ID != "high" {
		t.Errorf("pure relevance: expected high first, got %q", res.Scored[0].ID)
	}
}

func TestMathReranker_LambdaPositive_MMRApplied_MetricIncrements(t *testing.T) {
	// Read current counter value before.
	var before dto.Metric
	if err := rerankMathMMRAppliedTotal.Write(&before); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	beforeVal := before.Counter.GetValue()

	m := MathReranker{
		QueryVector: []float32{1, 0},
		Lambda:      0.5,
	}
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{1, 0}},
		{ID: "b", EmbedVector: []float32{0, 1}},
	}
	res, _ := m.RerankWithResult(context.Background(), "", docs)
	if res.Status != StatusOk {
		t.Fatalf("status: %v", res.Status)
	}

	// Check counter incremented by 1.
	var after dto.Metric
	if err := rerankMathMMRAppliedTotal.Write(&after); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	afterVal := after.Counter.GetValue()
	if afterVal-beforeVal < 1 {
		t.Errorf("MMR metric: want +1, got before=%v after=%v", beforeVal, afterVal)
	}
}

func TestMathReranker_SatisfiesRerankerInterface(t *testing.T) {
	var _ Reranker = MathReranker{}
}

func TestMathReranker_DryRunOpt_Passthrough(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{1, 0}},
		{ID: "b", EmbedVector: []float32{0, 1}},
	}
	res, err := m.RerankWithResult(context.Background(), "", docs, WithDryRun())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("dryrun: status got %v want Skipped", res.Status)
	}
	// Passthrough preserves original order.
	if len(res.Scored) != 2 {
		t.Fatalf("dryrun: len %d want 2", len(res.Scored))
	}
	if res.Scored[0].ID != "a" || res.Scored[1].ID != "b" {
		t.Errorf("dryrun: order not preserved: %q %q", res.Scored[0].ID, res.Scored[1].ID)
	}
}

func TestMathReranker_RerankWithResult_StatusOk(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{{ID: "a", EmbedVector: []float32{1, 0}}}
	res, err := m.RerankWithResult(context.Background(), "query", docs)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
}

func TestMathReranker_AvailableTrue_WhenQueryVectorPresent(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}}
	if !m.Available() {
		t.Error("Available() = false, want true when QueryVector is set")
	}
}

func TestMathReranker_AvailableFalse_WhenQueryVectorEmpty(t *testing.T) {
	m := MathReranker{}
	if m.Available() {
		t.Error("Available() = true, want false when QueryVector is empty")
	}
}

func TestMathReranker_EmptyVectorMetric_FiresOnMissingVecs(t *testing.T) {
	// Read current counter value before.
	var before dto.Metric
	if err := rerankMathEmptyVectorTotal.Write(&before); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	beforeVal := before.Counter.GetValue()

	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "no-vec-1"}, // missing EmbedVector
		{ID: "no-vec-2"}, // missing EmbedVector
		{ID: "has-vec", EmbedVector: []float32{1, 0}},
	}
	_, _ = m.RerankWithResult(context.Background(), "", docs)

	var after dto.Metric
	if err := rerankMathEmptyVectorTotal.Write(&after); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	afterVal := after.Counter.GetValue()

	if afterVal-beforeVal < 2 {
		t.Errorf("empty-vector metric: want >=2 increment, got before=%v after=%v", beforeVal, afterVal)
	}
}

// TestMathReranker_Rerank_Delegates ensures Rerank delegates to RerankWithResult
// and returns the same Scored slice.
func TestMathReranker_Rerank_Delegates(t *testing.T) {
	m := MathReranker{QueryVector: []float32{1, 0}, Lambda: 0}
	docs := []Doc{
		{ID: "a", EmbedVector: []float32{0, 1}},
		{ID: "b", EmbedVector: []float32{1, 0}},
	}
	scored := m.Rerank(context.Background(), "", docs)
	if len(scored) != 2 {
		t.Fatalf("Rerank: len %d want 2", len(scored))
	}
	if scored[0].ID != "b" {
		t.Errorf("top: got %q want b", scored[0].ID)
	}
}
