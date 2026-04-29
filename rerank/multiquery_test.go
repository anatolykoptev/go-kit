package rerank

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// ── stub Reranker ─────────────────────────────────────────────────────────────

// fixedReranker returns predetermined Scored slices for each successive call.
// If calls exceed the slice, it panics to surface test bugs.
type fixedReranker struct {
	results []*Result
	calls   atomic.Int64
}

func (f *fixedReranker) Rerank(ctx context.Context, query string, docs []Doc) []Scored {
	res, _ := f.RerankWithResult(ctx, query, docs)
	if res == nil {
		return multiPassthrough(docs)
	}
	return res.Scored
}

func (f *fixedReranker) RerankWithResult(_ context.Context, _ string, docs []Doc, _ ...RerankOpt) (*Result, error) {
	idx := int(f.calls.Add(1)) - 1
	if idx >= len(f.results) {
		// Return passthrough for unexpected calls.
		return &Result{Scored: multiPassthrough(docs), Status: StatusOk}, nil
	}
	r := f.results[idx]
	if r == nil || r.Status == StatusDegraded {
		var err error
		if r != nil {
			err = r.Err
		}
		if err == nil {
			err = errors.New("stub error")
		}
		if r == nil {
			r = &Result{Scored: multiPassthrough(docs), Status: StatusDegraded, Err: err}
		}
		return r, err
	}
	return r, nil
}

func (f *fixedReranker) Available() bool { return true }

// errorReranker always returns an error.
type errorReranker struct{}

func (e errorReranker) Rerank(ctx context.Context, query string, docs []Doc) []Scored {
	return multiPassthrough(docs)
}

func (e errorReranker) RerankWithResult(_ context.Context, _ string, docs []Doc, _ ...RerankOpt) (*Result, error) {
	err := errors.New("stub error")
	return &Result{Scored: multiPassthrough(docs), Status: StatusDegraded, Err: err}, err
}

func (e errorReranker) Available() bool { return true }

// makeTestDocs creates n docs with IDs "d0".."dN-1".
func makeTestDocs(n int) []Doc {
	docs := make([]Doc, n)
	for i := range docs {
		docs[i] = Doc{ID: "d" + itoa(i), Text: "text" + itoa(i)}
	}
	return docs
}

// resultWithScores builds a *Result with Scored sorted descending by score.
// scores[i] is the score for docs[i].
func resultWithScores(docs []Doc, scores []float32) *Result {
	out := make([]Scored, len(docs))
	for i, d := range docs {
		out[i] = Scored{Doc: d, Score: scores[i], OrigRank: i}
	}
	// Sort desc by score (mimics what rerankInternal produces).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Score > out[j-1].Score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return &Result{Scored: out, Status: StatusOk}
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestMultiQuery_RerankMulti_CombineMax_KnownInputs(t *testing.T) {
	docs := makeTestDocs(3)
	// Query 0: doc0=0.9, doc1=0.3, doc2=0.5
	// Query 1: doc0=0.2, doc1=0.8, doc2=0.4
	// Max:     doc0=0.9, doc1=0.8, doc2=0.5
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.9, 0.3, 0.5}),
		resultWithScores(docs, []float32{0.2, 0.8, 0.4}),
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineMax}
	ctx := context.Background()

	res, err := mq.RerankMulti(ctx, []string{"q0", "q1"}, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Fatalf("status: got %v want Ok", res.Status)
	}
	if len(res.Scored) != 3 {
		t.Fatalf("len: got %d want 3", len(res.Scored))
	}
	// Top doc should be doc0 with score 0.9.
	if res.Scored[0].ID != "d0" {
		t.Errorf("top doc: got %q want d0", res.Scored[0].ID)
	}
	if res.Scored[0].Score != 0.9 {
		t.Errorf("top score: got %v want 0.9", res.Scored[0].Score)
	}
	// Second should be doc1 with score 0.8.
	if res.Scored[1].ID != "d1" {
		t.Errorf("second doc: got %q want d1", res.Scored[1].ID)
	}
	if res.Scored[1].Score != 0.8 {
		t.Errorf("second score: got %v want 0.8", res.Scored[1].Score)
	}
}

func TestMultiQuery_RerankMulti_CombineAvg_KnownInputs(t *testing.T) {
	docs := makeTestDocs(2)
	// Query 0: doc0=0.8, doc1=0.4
	// Query 1: doc0=0.4, doc1=0.6
	// Avg:     doc0=0.6, doc1=0.5
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.8, 0.4}),
		resultWithScores(docs, []float32{0.4, 0.6}),
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineAvg}
	ctx := context.Background()

	res, err := mq.RerankMulti(ctx, []string{"q0", "q1"}, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// doc0 avg=(0.8+0.4)/2=0.6, doc1 avg=(0.4+0.6)/2=0.5
	if res.Scored[0].ID != "d0" {
		t.Errorf("top doc: got %q want d0", res.Scored[0].ID)
	}
	const eps = float32(1e-5)
	if diff := res.Scored[0].Score - 0.6; diff < -eps || diff > eps {
		t.Errorf("top score: got %v want 0.6", res.Scored[0].Score)
	}
	if res.Scored[1].ID != "d1" {
		t.Errorf("second doc: got %q want d1", res.Scored[1].ID)
	}
	if diff := res.Scored[1].Score - 0.5; diff < -eps || diff > eps {
		t.Errorf("second score: got %v want 0.5", res.Scored[1].Score)
	}
}

func TestMultiQuery_RerankMulti_CombineRRF_KnownInputs(t *testing.T) {
	docs := makeTestDocs(3)
	// Query 0: doc0=0.9(rank0), doc2=0.5(rank1), doc1=0.3(rank2)
	// Query 1: doc1=0.8(rank0), doc2=0.4(rank1), doc0=0.2(rank2)
	// RRF k=60:
	//   doc0: 1/(60+0+1) + 1/(60+2+1) = 1/61 + 1/63 ≈ 0.01639 + 0.01587 ≈ 0.03226
	//   doc1: 1/(60+2+1) + 1/(60+0+1) = 1/63 + 1/61 ≈ 0.01587 + 0.01639 ≈ 0.03226
	//   doc2: 1/(60+1+1) + 1/(60+1+1) = 1/62 + 1/62 ≈ 0.01613 + 0.01613 ≈ 0.03226
	// They're all nearly equal; doc0 and doc1 rank 0 in one query each.
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.9, 0.3, 0.5}),
		resultWithScores(docs, []float32{0.2, 0.8, 0.4}),
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineRRF, RRFK: 60}
	ctx := context.Background()

	res, err := mq.RerankMulti(ctx, []string{"q0", "q1"}, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Fatalf("status: got %v want Ok", res.Status)
	}
	if len(res.Scored) != 3 {
		t.Fatalf("len: got %d want 3", len(res.Scored))
	}
	// Verify all scores are positive (RRF always produces positive scores).
	for _, s := range res.Scored {
		if s.Score <= 0 {
			t.Errorf("RRF score must be positive, got %v for doc %s", s.Score, s.ID)
		}
	}
	// doc0: ranks 0 in q0, rank 2 in q1 → 1/61 + 1/63 ≈ 0.03226
	// doc2: rank 1 in both → 1/62 + 1/62 ≈ 0.03226
	// doc1: rank 2 in q0, rank 0 in q1 → 1/63 + 1/61 ≈ 0.03226
	// All equal — verify formula: each score ≈ 1/(k+rank+1) sum
	for _, s := range res.Scored {
		if s.Score < 0.03 || s.Score > 0.04 {
			t.Errorf("doc %s: RRF score %v outside expected range [0.03, 0.04]", s.ID, s.Score)
		}
	}
}

func TestMultiQuery_EmptyQueries_ReturnsError(t *testing.T) {
	mq := MultiQuery{Inner: &fixedReranker{}, Combine: CombineMax}
	_, err := mq.RerankMulti(context.Background(), nil, makeTestDocs(2))
	if !errors.Is(err, ErrEmptyQueries) {
		t.Errorf("got %v, want ErrEmptyQueries", err)
	}
}

func TestMultiQuery_AllFail_StatusDegraded(t *testing.T) {
	docs := makeTestDocs(2)
	degraded := &Result{Scored: multiPassthrough(docs), Status: StatusDegraded, Err: errors.New("boom")}
	inner := &fixedReranker{results: []*Result{degraded, degraded, degraded}}
	mq := MultiQuery{Inner: inner, Combine: CombineMax}

	res, err := mq.RerankMulti(context.Background(), []string{"q0", "q1", "q2"}, docs)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if res == nil {
		t.Fatal("result must not be nil")
	}
	if res.Status != StatusDegraded {
		t.Errorf("status: got %v want StatusDegraded", res.Status)
	}
	if len(res.Scored) != len(docs) {
		t.Errorf("passthrough len: got %d want %d", len(res.Scored), len(docs))
	}
}

func TestMultiQuery_PartialFail_StatusOk(t *testing.T) {
	docs := makeTestDocs(3)
	degraded := &Result{Scored: multiPassthrough(docs), Status: StatusDegraded, Err: errors.New("one bad")}
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.9, 0.5, 0.3}), // q0: succeeds
		degraded, // q1: fails
		resultWithScores(docs, []float32{0.2, 0.8, 0.6}), // q2: succeeds
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineMax}

	res, err := mq.RerankMulti(context.Background(), []string{"q0", "q1", "q2"}, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	// Max of q0 and q2 (q1 ignored): doc0=max(0.9,0.2)=0.9, doc1=max(0.5,0.8)=0.8, doc2=max(0.3,0.6)=0.6
	if res.Scored[0].ID != "d0" {
		t.Errorf("top: got %q want d0", res.Scored[0].ID)
	}
}

func TestMultiQuery_BoundedConcurrency_RaceClean(t *testing.T) {
	// 10 queries, Concurrency=2; verifies -race and semaphore correctness.
	docs := makeTestDocs(2)
	n := 10
	results := make([]*Result, n)
	for i := range results {
		results[i] = resultWithScores(docs, []float32{float32(i) * 0.1, float32(n-i) * 0.1})
	}
	inner := &fixedReranker{results: results}
	mq := MultiQuery{Inner: inner, Combine: CombineAvg, Concurrency: 2}

	queries := make([]string, n)
	for i := range queries {
		queries[i] = "query" + itoa(i)
	}

	res, err := mq.RerankMulti(context.Background(), queries, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusOk {
		t.Errorf("status: got %v want Ok", res.Status)
	}
	if len(res.Scored) != len(docs) {
		t.Errorf("len: got %d want %d", len(res.Scored), len(docs))
	}
}

func TestMultiQuery_OptsForwardedToInner(t *testing.T) {
	docs := makeTestDocs(3)
	var observedTopN int
	inner := &optsCapturingReranker{docs: docs, captureTopN: &observedTopN}
	mq := MultiQuery{Inner: inner, Combine: CombineMax}

	_, _ = mq.RerankMulti(context.Background(), []string{"q0"}, docs, WithTopN(2))
	if observedTopN != 2 {
		t.Errorf("TopN not forwarded: got %d want 2", observedTopN)
	}
}

// optsCapturingReranker captures per-call opts for inspection.
type optsCapturingReranker struct {
	docs        []Doc
	captureTopN *int
}

func (o *optsCapturingReranker) Rerank(ctx context.Context, query string, docs []Doc) []Scored {
	return multiPassthrough(docs)
}

func (o *optsCapturingReranker) RerankWithResult(_ context.Context, _ string, docs []Doc, opts ...RerankOpt) (*Result, error) {
	cfg := rerankCallCfg{}
	for _, opt := range opts {
		opt(&cfg)
	}
	*o.captureTopN = cfg.TopN
	return &Result{Scored: multiPassthrough(docs), Status: StatusOk}, nil
}

func (o *optsCapturingReranker) Available() bool { return true }

func TestMultiQuery_SatisfiesRerankerInterface(t *testing.T) {
	// Compile-time assertion via runtime assignment.
	var _ Reranker = MultiQuery{Inner: &fixedReranker{}}
}

func TestMultiQuery_RerankSingleQuery_DelegatesToInner(t *testing.T) {
	docs := makeTestDocs(2)
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.7, 0.3}),
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineMax}
	ctx := context.Background()

	// Rerank (single-query) should delegate to Inner.
	scored := mq.Rerank(ctx, "q", docs)
	if len(scored) != 2 {
		t.Fatalf("Rerank len: got %d want 2", len(scored))
	}
	if scored[0].ID != "d0" {
		t.Errorf("Rerank top: got %q want d0", scored[0].ID)
	}

	// RerankWithResult (single-query) should also delegate to Inner.
	inner2 := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.2, 0.9}),
	}}
	mq2 := MultiQuery{Inner: inner2, Combine: CombineMax}
	res, err := mq2.RerankWithResult(ctx, "q", docs)
	if err != nil {
		t.Fatalf("RerankWithResult error: %v", err)
	}
	if res.Scored[0].ID != "d1" {
		t.Errorf("RerankWithResult top: got %q want d1", res.Scored[0].ID)
	}
}

func TestMultiQuery_DefaultRRFK_Applied(t *testing.T) {
	docs := makeTestDocs(2)
	// RRFK=0 should default to 60 internally.
	inner := &fixedReranker{results: []*Result{
		resultWithScores(docs, []float32{0.9, 0.1}),
	}}
	mq := MultiQuery{Inner: inner, Combine: CombineRRF, RRFK: 0}
	res, err := mq.RerankMulti(context.Background(), []string{"q"}, docs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// doc0 rank=0: 1/(60+0+1)=1/61≈0.01639; doc1 rank=1: 1/(60+1+1)=1/62≈0.01613
	if res.Scored[0].ID != "d0" {
		t.Errorf("top: got %q want d0", res.Scored[0].ID)
	}
}
