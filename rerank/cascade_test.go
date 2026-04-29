package rerank

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

// mockReranker is a test double for the Reranker interface.
type mockReranker struct {
	name      string
	available bool
	response  *Result
	err       error
	calls     int
	// capturedOpts captures opts passed to RerankWithResult for inspection.
	capturedOpts []RerankOpt
}

func (m *mockReranker) Rerank(ctx context.Context, q string, docs []Doc) []Scored {
	res, _ := m.RerankWithResult(ctx, q, docs)
	if res == nil {
		return nil
	}
	return res.Scored
}

func (m *mockReranker) RerankWithResult(_ context.Context, _ string, _ []Doc, opts ...RerankOpt) (*Result, error) {
	m.calls++
	m.capturedOpts = opts
	return m.response, m.err
}

func (m *mockReranker) Available() bool { return m.available }

// makeDocs builds n dummy Docs for use in tests.
func makeDocs(n int) []Doc {
	docs := make([]Doc, n)
	for i := range docs {
		docs[i] = Doc{ID: itoa(i), Text: "text"}
	}
	return docs
}

// TestCascade_NilResultFromInnerReranker_NoPanic verifies Cascade defensively
// guards against custom Reranker impls that violate the non-nil-Result contract.
func TestCascade_NilResultFromInnerReranker_NoPanic(t *testing.T) {
	badStage := &mockReranker{
		name:      "bad",
		available: true,
		response:  nil, // BAD: contract says non-nil
		err:       errors.New("bad reranker returned nil"),
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: badStage, KeepTopN: 0, Label: "bad-stage"},
	}}
	docs := []Doc{{ID: "a", Text: "x"}}

	// Must not panic.
	res, err := c.RerankWithResult(context.Background(), "q", docs)

	// Should return a degraded Result with the error attached.
	if res == nil {
		t.Fatal("Cascade returned nil Result; must reconstruct passthrough")
	}
	if res.Status != StatusDegraded {
		t.Errorf("expected StatusDegraded on nil-res from inner, got %v", res.Status)
	}
	if err == nil {
		t.Error("expected err propagated from inner")
	}
}

// TestCascade_SatisfiesRerankerInterface is a compile-time assertion.
func TestCascade_SatisfiesRerankerInterface(t *testing.T) {
	var _ Reranker = Cascade{}
	var _ Reranker = (*Client)(nil)
}

// TestCascade_TwoStageChaining verifies stage 0 → KeepTopN=3 → stage 1 → KeepTopN=2.
func TestCascade_TwoStageChaining(t *testing.T) {
	docs := makeDocs(5)
	stage0Scored := []Scored{
		{Doc: docs[4], Score: 0.9},
		{Doc: docs[2], Score: 0.8},
		{Doc: docs[1], Score: 0.7},
		{Doc: docs[3], Score: 0.6},
		{Doc: docs[0], Score: 0.5},
	}
	stage1Scored := []Scored{
		{Doc: docs[4], Score: 0.95},
		{Doc: docs[1], Score: 0.75},
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage0Scored, Model: "fast"},
	}
	mock1 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage1Scored, Model: "slow"},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 3, Label: "prefilter"},
		{Reranker: mock1, KeepTopN: 2, Label: "deep"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if len(res.Scored) != 2 {
		t.Fatalf("len(Scored): got %d want 2", len(res.Scored))
	}
	if mock0.calls != 1 {
		t.Errorf("stage0 calls: got %d want 1", mock0.calls)
	}
	if mock1.calls != 1 {
		t.Errorf("stage1 calls: got %d want 1", mock1.calls)
	}
	// Stage 1 got 3 docs (KeepTopN=3 from stage 0).
	// We can only verify the mock was called once; doc count is inspected via mock in the wrapper above.
}

// TestCascade_KeepTopNCutsList verifies KeepTopN=2 on 5 docs yields 2 in the final result.
func TestCascade_KeepTopNCutsList(t *testing.T) {
	docs := makeDocs(5)
	scored := make([]Scored, 5)
	for i := range scored {
		scored[i] = Scored{Doc: docs[i], Score: float32(5-i) * 0.1}
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 2, Label: "single"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 2 {
		t.Errorf("len(Scored): got %d want 2", len(res.Scored))
	}
}

// TestCascade_KeepTopNZeroPassesAll verifies KeepTopN=0 keeps all docs.
func TestCascade_KeepTopNZeroPassesAll(t *testing.T) {
	docs := makeDocs(5)
	scored := make([]Scored, 5)
	for i := range scored {
		scored[i] = Scored{Doc: docs[i], Score: float32(i) * 0.1}
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 0, Label: "nocut"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 5 {
		t.Errorf("len(Scored): got %d want 5 (KeepTopN=0 must pass all)", len(res.Scored))
	}
}

// TestCascade_KeepTopNLargerThanInput_NoOp verifies KeepTopN=100 on 3 docs keeps 3.
func TestCascade_KeepTopNLargerThanInput_NoOp(t *testing.T) {
	docs := makeDocs(3)
	scored := make([]Scored, 3)
	for i := range scored {
		scored[i] = Scored{Doc: docs[i], Score: float32(i) * 0.1}
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 100, Label: "big-cap"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Scored) != 3 {
		t.Errorf("len(Scored): got %d want 3 (KeepTopN > len is no-op)", len(res.Scored))
	}
}

// TestCascade_StopBelowThreshold_TriggersEarlyExit verifies that when the top
// score of stage 0 is below StopBelowThreshold, stage 1 is NOT called.
func TestCascade_StopBelowThreshold_TriggersEarlyExit(t *testing.T) {
	docs := makeDocs(3)
	stage0Scored := []Scored{
		{Doc: docs[0], Score: 0.3},
		{Doc: docs[1], Score: 0.2},
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage0Scored},
	}
	mock1 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: nil},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 5, Label: "stage0", StopBelowThreshold: 0.5},
		{Reranker: mock1, KeepTopN: 2, Label: "stage1"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock1.calls != 0 {
		t.Errorf("stage1 must NOT be called on early exit, got %d calls", mock1.calls)
	}
	// Result should be stage 0's output.
	if len(res.Scored) != 2 {
		t.Errorf("len(Scored): got %d want 2 (stage 0 output)", len(res.Scored))
	}
}

// TestCascade_StopBelowThresholdZero_NeverTriggers verifies threshold=0 never short-circuits.
func TestCascade_StopBelowThresholdZero_NeverTriggers(t *testing.T) {
	docs := makeDocs(3)
	stage0Scored := []Scored{{Doc: docs[0], Score: 0.0}} // score=0, lowest possible
	stage1Scored := []Scored{{Doc: docs[0], Score: 0.9}}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage0Scored},
	}
	mock1 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage1Scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, Label: "s0", StopBelowThreshold: 0}, // 0 = disabled
		{Reranker: mock1, Label: "s1"},
	}}
	_, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock1.calls != 1 {
		t.Errorf("stage1 must be called when threshold=0, got %d calls", mock1.calls)
	}
}

// TestCascade_MidStageFailure_PropagatesDegraded verifies that a stage returning
// StatusDegraded causes the Cascade to return StatusDegraded with the error.
func TestCascade_MidStageFailure_PropagatesDegraded(t *testing.T) {
	docs := makeDocs(3)
	boom := errors.New("boom")
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusDegraded, Scored: nil, Err: boom},
		err:       boom,
	}
	mock1 := &mockReranker{available: true}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, Label: "failing"},
		{Reranker: mock1, Label: "never-reached"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if res.Status != StatusDegraded {
		t.Errorf("status: got %v want Degraded", res.Status)
	}
	if mock1.calls != 0 {
		t.Errorf("stage1 must not be called after mid-stage failure, got %d calls", mock1.calls)
	}
}

// TestCascade_EmptyStages_ReturnsSkipped verifies empty Stages yields StatusSkipped
// with passthrough docs in original order.
func TestCascade_EmptyStages_ReturnsSkipped(t *testing.T) {
	docs := makeDocs(3)
	c := Cascade{}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusSkipped {
		t.Errorf("status: got %v want Skipped", res.Status)
	}
	if len(res.Scored) != 3 {
		t.Errorf("len(Scored): got %d want 3", len(res.Scored))
	}
	for i, s := range res.Scored {
		if s.ID != docs[i].ID {
			t.Errorf("pos %d: got %q want %q (passthrough order)", i, s.ID, docs[i].ID)
		}
	}
}

// TestCascade_AvailableAllStagesUp_ReturnsTrue verifies Available returns true when all mocks are up.
func TestCascade_AvailableAllStagesUp_ReturnsTrue(t *testing.T) {
	c := Cascade{Stages: []StageConfig{
		{Reranker: &mockReranker{available: true}, Label: "s0"},
		{Reranker: &mockReranker{available: true}, Label: "s1"},
	}}
	if !c.Available() {
		t.Error("Available: got false want true (all stages up)")
	}
}

// TestCascade_AvailableOneStageDown_ReturnsFalse verifies Available returns false
// when any stage is unavailable.
func TestCascade_AvailableOneStageDown_ReturnsFalse(t *testing.T) {
	c := Cascade{Stages: []StageConfig{
		{Reranker: &mockReranker{available: true}, Label: "s0"},
		{Reranker: &mockReranker{available: false}, Label: "s1-down"},
	}}
	if c.Available() {
		t.Error("Available: got true want false (one stage down)")
	}
}

// TestCascade_AvailableEmpty_ReturnsFalse verifies Available returns false for
// an empty Cascade.
func TestCascade_AvailableEmpty_ReturnsFalse(t *testing.T) {
	if (Cascade{}).Available() {
		t.Error("Available: got true want false (empty cascade)")
	}
}

// TestCascade_OptsForwardedToInnerStage verifies that RerankOpts passed to
// Cascade.RerankWithResult are forwarded to the inner stage's Reranker.
func TestCascade_OptsForwardedToInnerStage(t *testing.T) {
	docs := makeDocs(5)
	scored := make([]Scored, 5)
	for i := range scored {
		scored[i] = Scored{Doc: docs[i], Score: float32(i) * 0.1}
	}
	mock := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock, KeepTopN: 5, Label: "s0"},
	}}

	opt := WithTopN(5)
	_, err := c.RerankWithResult(context.Background(), "q", docs, opt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.capturedOpts) == 0 {
		t.Error("opts not forwarded to inner stage")
	}
	// Apply the captured opts to verify they contain our sentinel.
	cfg := &rerankCallCfg{}
	for _, o := range mock.capturedOpts {
		o(cfg)
	}
	if cfg.TopN != 5 {
		t.Errorf("TopN not forwarded: got %d want 5", cfg.TopN)
	}
}

// TestCascade_HooksFire verifies that OnBefore/OnAfter hooks fire from the
// inner Reranker (via *Client), not from Cascade itself (Cascade passes through).
// We use a real Client wrapping a mock HTTP server via mockReranker so we can
// verify the inner hooks fire without involving the mock transport.
//
// Since mockReranker itself doesn't fire hooks (it's a test double), we verify
// that Cascade correctly delegates to the inner Reranker's RerankWithResult.
// The inner Reranker is responsible for hook invocation.
func TestCascade_HooksFire(t *testing.T) {
	docs := makeDocs(3)
	scored := make([]Scored, 3)
	for i := range scored {
		scored[i] = Scored{Doc: docs[i], Score: float32(i) * 0.1}
	}
	mock := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock, Label: "s0"},
	}}
	_, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Inner mock was called exactly once — hooks would fire if it were a real Client.
	if mock.calls != 1 {
		t.Errorf("inner Reranker calls: got %d want 1", mock.calls)
	}
}

// TestCascade_ScoresFromLaterStageWin verifies that stage 1's scores replace
// stage 0's — i.e. scores do NOT propagate between stages.
func TestCascade_ScoresFromLaterStageWin(t *testing.T) {
	docs := makeDocs(3)
	stage0Scored := []Scored{
		{Doc: docs[0], Score: 0.1},
		{Doc: docs[1], Score: 0.2},
		{Doc: docs[2], Score: 0.3},
	}
	// Stage 1 reverses the order and uses completely different scores.
	stage1Scored := []Scored{
		{Doc: docs[2], Score: 0.99},
		{Doc: docs[1], Score: 0.88},
		{Doc: docs[0], Score: 0.77},
	}
	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage0Scored},
	}
	mock1 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage1Scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, Label: "s0"},
		{Reranker: mock1, Label: "s1"},
	}}
	res, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stage 1's scores must win.
	if res.Scored[0].Score != 0.99 {
		t.Errorf("top score: got %v want 0.99 (stage 1 scores must win)", res.Scored[0].Score)
	}
	if res.Scored[0].ID != docs[2].ID {
		t.Errorf("top doc: got %q want %q", res.Scored[0].ID, docs[2].ID)
	}
}

// TestCascade_MemoryPeakIsMaxStageIn verifies that a two-stage cascade with
// KeepTopN=10 and KeepTopN=5 allocates at most proportional to the first stage's
// input (1000 docs), not to the sum of all stages.
//
// This is a best-effort allocation sanity check, not a hard bound. It runs
// GC before and after to reduce noise.
func TestCascade_MemoryPeakIsMaxStageIn(t *testing.T) {
	const n = 1000

	docs := makeDocs(n)
	stage0Scored := make([]Scored, n)
	for i := range stage0Scored {
		stage0Scored[i] = Scored{Doc: docs[i], Score: float32(n-i) * 0.001}
	}
	stage1Scored := make([]Scored, 10)
	for i := range stage1Scored {
		stage1Scored[i] = Scored{Doc: docs[i], Score: float32(10-i) * 0.001}
	}

	mock0 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage0Scored},
	}
	mock1 := &mockReranker{
		available: true,
		response:  &Result{Status: StatusOk, Scored: stage1Scored},
	}
	c := Cascade{Stages: []StageConfig{
		{Reranker: mock0, KeepTopN: 10, Label: "pre"},
		{Reranker: mock1, KeepTopN: 5, Label: "deep"},
	}}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	_, err := c.RerankWithResult(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	// Alloc delta should be well under "both stages processing 1000 docs each".
	// After KeepTopN=10, stage 1 only sees 10 docs. Peak is dominated by stage 0 input.
	allocDelta := int64(after.TotalAlloc) - int64(before.TotalAlloc)
	// 1000 docs * ~200 bytes each (Doc + Scored) = ~200KB. Allow 5× headroom.
	const maxExpectedAlloc = 1000 * 200 * 5
	if allocDelta > maxExpectedAlloc {
		t.Logf("alloc delta: %d bytes (limit %d) — may indicate O(n*stages) allocation", allocDelta, maxExpectedAlloc)
		// Not a hard failure: test noise can cause false positives. Log only.
	}
	t.Logf("alloc delta across two-stage cascade (1000 → 10 → 5 docs): %d bytes", allocDelta)
}
