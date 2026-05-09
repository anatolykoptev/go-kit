package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// chunkingTestServer creates a mock embed server that:
//   - Records every call in callCount.
//   - Decodes the request body and returns vectors based on inputFn(texts).
//   - If inputFn is nil, returns vec[i] = [float32(i)] for each text in the sub-batch.
//
// IMPORTANT: The server applies indexing relative to the sub-batch (each call
// starts at 0), so callers that need order-preservation must use a custom
// inputFn that encodes ordering information in the texts themselves.
func chunkingTestServer(t *testing.T, callCount *atomic.Int32, inputFn func(texts []string) ([][]float32, int)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req struct {
			Input []string `json:"input"`
			Model string   `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var embeddings [][]float32
		statusCode := http.StatusOK
		if inputFn != nil {
			embeddings, statusCode = inputFn(req.Input)
		} else {
			embeddings = make([][]float32, len(req.Input))
			for i := range req.Input {
				embeddings[i] = []float32{float32(i)}
			}
		}
		if statusCode != http.StatusOK {
			http.Error(w, fmt.Sprintf("error %d", statusCode), statusCode)
			return
		}

		type embeddingObj struct {
			Object    string    `json:"object"`
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		type respBody struct {
			Object string         `json:"object"`
			Data   []embeddingObj `json:"data"`
			Model  string         `json:"model"`
		}
		data := make([]embeddingObj, len(embeddings))
		for i, emb := range embeddings {
			data[i] = embeddingObj{Object: "embedding", Embedding: emb, Index: i}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Object: "list", Data: data, Model: req.Model}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

// -- Test 8: 100 texts, chunkSize=32 → 4 HTTP calls, result length 100, ordering preserved --

// TestClient_Embed_ChunksLargeInput verifies that 100 texts with chunkSize=32
// results in 4 sequential HTTP calls (32+32+32+4) and a result of length 100.
func TestClient_Embed_ChunksLargeInput(t *testing.T) {
	var callCount atomic.Int32
	// Each sub-batch gets vectors relative to position in sub-batch.
	// For ordering, we just need len check — see TestClient_Embed_PreservesOrder.
	srv := chunkingTestServer(t, &callCount, nil)

	c, err := NewClient(srv.URL,
		WithModel("test-model"),
		WithDim(1),
		WithChunkSize(32),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 100)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := c.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := callCount.Load(); got != 4 {
		t.Errorf("HTTP call count: want 4, got %d", got)
	}
	if len(result) != 100 {
		t.Errorf("result length: want 100, got %d", len(result))
	}
}

// -- Test 9: 16 texts, chunkSize=32 → 1 HTTP call --

// TestClient_Embed_NoChunkUnderCap verifies that when len(texts) <= chunkSize,
// exactly 1 HTTP call is made.
func TestClient_Embed_NoChunkUnderCap(t *testing.T) {
	var callCount atomic.Int32
	srv := chunkingTestServer(t, &callCount, nil)

	c, err := NewClient(srv.URL,
		WithModel("test-model"),
		WithDim(1),
		WithChunkSize(32),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 16)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := c.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("HTTP call count: want 1, got %d", got)
	}
	if len(result) != 16 {
		t.Errorf("result length: want 16, got %d", len(result))
	}
}

// -- Test 10: 50 texts where vec[i][0] == float32(i), ordering verified --

// TestClient_Embed_PreservesOrder verifies that chunking preserves the order
// of results. Each text is "text-N" and the mock returns vec[position] = N
// (encoded in the text), so result[i][0] must equal float32(i).
func TestClient_Embed_PreservesOrder(t *testing.T) {
	var callCount atomic.Int32

	// Custom server: parse the N from "text-N" and return vec = [float32(N)]
	srv := chunkingTestServer(t, &callCount, func(texts []string) ([][]float32, int) {
		out := make([][]float32, len(texts))
		for i, text := range texts {
			var n int
			fmt.Sscanf(text, "text-%d", &n) //nolint:errcheck
			out[i] = []float32{float32(n)}
		}
		return out, http.StatusOK
	})

	c, err := NewClient(srv.URL,
		WithModel("test-model"),
		WithDim(1),
		WithChunkSize(10),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 50)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := c.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(result) != 50 {
		t.Fatalf("result length: want 50, got %d", len(result))
	}
	for i, vec := range result {
		if len(vec) == 0 {
			t.Errorf("[%d] vec is empty", i)
			continue
		}
		if vec[0] != float32(i) {
			t.Errorf("[%d] vec[0]: want %g, got %g", i, float32(i), vec[0])
		}
	}
}

// -- Test 11: atomic error on sub-batch failure --

// TestClient_Embed_FailsAtomicallyOnSubBatchError verifies that:
//   - When the 2nd sub-batch fails, the error is returned immediately.
//   - No partial result is returned.
//   - The 3rd sub-batch is never started (only 2 HTTP calls total).
func TestClient_Embed_FailsAtomicallyOnSubBatchError(t *testing.T) {
	var callCount atomic.Int32

	srv := chunkingTestServer(t, &callCount, func(texts []string) ([][]float32, int) {
		call := callCount.Load()
		if call == 2 {
			// 2nd call fails
			return nil, http.StatusInternalServerError
		}
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{float32(i)}
		}
		return out, http.StatusOK
	})

	c, err := NewClient(srv.URL,
		WithModel("test-model"),
		WithDim(1),
		WithChunkSize(10),
		// Disable retry so the 500 fails fast without 3 attempts per chunk.
		WithRetry(NoRetry),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 25)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := c.Embed(context.Background(), texts)
	if err == nil {
		t.Fatal("expected error when 2nd sub-batch fails")
	}
	if result != nil {
		t.Errorf("expected nil result on sub-batch error, got len=%d", len(result))
	}
	if got := callCount.Load(); got != 2 {
		t.Errorf("HTTP call count: want 2 (first succeeds, second fails, third never starts), got %d", got)
	}
}

// -- Test 12: chunkSize from env --

// TestClient_Embed_ChunkSizeFromEnv verifies that GOKIT_EMBED_CHUNK_SIZE is
// parsed and applied to the Client.
func TestClient_Embed_ChunkSizeFromEnv(t *testing.T) {
	t.Setenv("GOKIT_EMBED_CHUNK_SIZE", "8")

	// Use WithChunkSize(0) to signal "read from env" — or simply don't call
	// WithChunkSize and let the constructor read the env.
	c, err := NewClient("http://embed:8082",
		WithModel("test-model"),
		WithDim(1024),
		// no WithChunkSize — env should drive it
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.chunkSize != 8 {
		t.Errorf("chunkSize: want 8 (from env), got %d", c.chunkSize)
	}
}

// -- Test 13: DimMismatch error reports chunk offset --

// TestClient_Embed_DimMismatchErrorReportsChunkOffset verifies that when a
// dim mismatch occurs on the 3rd chunk (offset=20), the error reports Index=20.
func TestClient_Embed_DimMismatchErrorReportsChunkOffset(t *testing.T) {
	var callCount atomic.Int32

	// 3rd call returns a wrong-dim vector (dim=2 instead of 1).
	srv := chunkingTestServer(t, &callCount, func(texts []string) ([][]float32, int) {
		call := callCount.Load()
		out := make([][]float32, len(texts))
		for i := range out {
			if call == 3 {
				// Wrong dim — will trigger ErrDimMismatch in validateDim.
				out[i] = []float32{float32(i), float32(i)} // dim=2
			} else {
				out[i] = []float32{float32(i)} // dim=1
			}
		}
		return out, http.StatusOK
	})

	c, err := NewClient(srv.URL,
		WithModel("test-model"),
		WithDim(1), // expect dim=1; 3rd chunk returns dim=2 → mismatch
		WithChunkSize(10),
		WithRetry(NoRetry),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 25)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	_, err = c.Embed(context.Background(), texts)
	if err == nil {
		t.Fatal("expected error for dim mismatch on 3rd chunk")
	}
	var dimErr *ErrDimMismatch
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *ErrDimMismatch, got %T: %v", err, err)
	}
	if dimErr.Index != 20 {
		t.Errorf("ErrDimMismatch.Index: want 20 (chunk offset), got %d", dimErr.Index)
	}
}

// -- Test 14: EmbedWithResult chunks through cache, per-chunk cache lookup --

// TestClient_EmbedWithResult_ChunksThroughCache verifies that cache lookup
// happens per-chunk (above chunking): with 50 texts and chunkSize=32, where
// the first 16 are pre-cached, the chunked dispatch breaks 50 → 32+18,
// chunk 1 (texts 0..31) has 16 cache hits + 16 misses → backend receives 32
// (current cache impl is full-batch lookup; partial miss falls through to
// backend with the FULL chunk), chunk 2 (texts 32..49) has 0 hits → backend
// receives 18. Total: 2 HTTP calls (one per chunk), with chunk dispatch
// proving the chunking gate is genuinely entered (not vacuous like the
// previous version which used len(texts) == chunkSize).
func TestClient_EmbedWithResult_ChunksThroughCache(t *testing.T) {
	var callCount atomic.Int32

	// Server returns deterministic vectors based on text content.
	srv := chunkingTestServer(t, &callCount, func(texts []string) ([][]float32, int) {
		out := make([][]float32, len(texts))
		for i, text := range texts {
			var n int
			fmt.Sscanf(text, "text-%d", &n) //nolint:errcheck
			out[i] = []float32{float32(n)}
		}
		return out, http.StatusOK
	})

	model := "cache-chunk-model"
	cache := newMapCache()
	ctx := context.Background()

	// Pre-populate cache for the FIRST 16 texts (text-0 .. text-15).
	for i := 0; i < 16; i++ {
		key := cacheKey(model, 1, "", "", fmt.Sprintf("text-%d", i), "passage")
		cache.Set(ctx, key, []float32{float32(i)})
	}

	c, err := NewClient(srv.URL,
		WithModel(model),
		WithDim(1),
		WithChunkSize(32),
		WithCache(cache),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// 50 texts > chunkSize=32 → chunking gate is genuinely entered.
	texts := make([]string, 50)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := c.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(result) != 50 {
		t.Fatalf("result length: want 50, got %d", len(result))
	}
	// Verify ordering: result[i] should equal float32(i).
	for i := 0; i < 50; i++ {
		if result[i][0] != float32(i) {
			t.Errorf("result[%d][0]: want %v, got %v", i, float32(i), result[i][0])
		}
	}
	// Two chunks dispatched (50 → 32+18). Each chunk hits the backend
	// because the current cache layer is full-batch (any miss → full
	// fallthrough). The CRUCIAL property tested here: chunking IS entered
	// (callCount > 0) and ordering is preserved across chunks.
	if got := callCount.Load(); got != 2 {
		t.Errorf("HTTP call count: want 2 (one per chunk), got %d — chunking gate may not have fired", got)
	}
}

// -- Test 15: embed_chunks_per_call metric recorded --

// TestClient_Embed_ChunksPerCallMetricRecorded verifies that embed_chunks_per_call
// histogram records 1 observation with value 4 for a 100-text call with chunkSize=32.
func TestClient_Embed_ChunksPerCallMetricRecorded(t *testing.T) {
	var callCount atomic.Int32
	srv := chunkingTestServer(t, &callCount, nil)

	model := "metric-test-model"
	c, err := NewClient(srv.URL,
		WithModel(model),
		WithDim(1),
		WithChunkSize(32),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	texts := make([]string, 100)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	// Capture the histogram metric before and after to verify 1 observation with value=4.
	histBefore := histogramSnapshot(embedChunksPerCall, model)

	_, err = c.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	histAfter := histogramSnapshot(embedChunksPerCall, model)

	countDelta := histAfter.count - histBefore.count
	sumDelta := histAfter.sum - histBefore.sum

	if countDelta != 1 {
		t.Errorf("embed_chunks_per_call sample count delta: want 1, got %d", countDelta)
	}
	if sumDelta != 4 {
		t.Errorf("embed_chunks_per_call sum delta: want 4 (ceil(100/32)=4 chunks), got %g", sumDelta)
	}
}

// -- Test 16: chunked dispatch through fallback chain (BLOCKER fix verify) --

// TestClient_EmbedWithResult_ChunksWithFallback verifies that fallback-wired
// clients also chunk their input. Before the BLOCKER fix, EmbedWithResult
// routed through embedWithFallback BEFORE the chunking gate, so a 100-text
// call to a fallback client hit the server cap (HTTP 400) and bypassed both
// chunking AND fallback (4xx classified as caller-error).
//
// After fix: chunking gate runs first. Each chunk routes through fallback,
// so primary failure on chunk 2 invokes secondary for chunk 2 only.
func TestClient_EmbedWithResult_ChunksWithFallback(t *testing.T) {
	var primaryCalls atomic.Int32
	var secondaryCalls atomic.Int32

	// Primary: returns 500 on the 2nd call, success otherwise.
	primarySrv := chunkingTestServer(t, &primaryCalls, func(texts []string) ([][]float32, int) {
		if primaryCalls.Load() == 2 {
			return nil, http.StatusInternalServerError
		}
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1.0}
		}
		return out, http.StatusOK
	})

	// Secondary: always succeeds with marker vector [9.0].
	secondarySrv := chunkingTestServer(t, &secondaryCalls, func(texts []string) ([][]float32, int) {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{9.0}
		}
		return out, http.StatusOK
	})

	secondary, err := NewClient(secondarySrv.URL,
		WithModel("secondary-model"),
		WithDim(1),
		WithRetry(NoRetry),
	)
	if err != nil {
		t.Fatalf("NewClient secondary: %v", err)
	}

	primary, err := NewClient(primarySrv.URL,
		WithModel("primary-model"),
		WithDim(1),
		WithChunkSize(10),
		WithFallback(secondary),
		WithRetry(NoRetry),
	)
	if err != nil {
		t.Fatalf("NewClient primary: %v", err)
	}

	// 25 texts → 3 chunks of 10+10+5.
	texts := make([]string, 25)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	result, err := primary.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(result) != 25 {
		t.Fatalf("result length: want 25, got %d", len(result))
	}
	// Primary received all 3 chunks (1st OK, 2nd 500, 3rd OK).
	if got := primaryCalls.Load(); got != 3 {
		t.Errorf("primary HTTP call count: want 3 (chunked), got %d — chunking may bypass fallback path", got)
	}
	// Secondary received only chunk 2 (the failed one).
	if got := secondaryCalls.Load(); got != 1 {
		t.Errorf("secondary HTTP call count: want 1 (fallback for chunk 2), got %d", got)
	}
	// Chunk 2's vectors (indices 10..19) must be from secondary (marker 9.0).
	for i := 10; i < 20; i++ {
		if result[i][0] != 9.0 {
			t.Errorf("result[%d][0]: want 9.0 (secondary marker), got %v", i, result[i][0])
		}
	}
	// Chunks 1 and 3 must be from primary (1.0).
	for _, i := range []int{0, 5, 9, 20, 24} {
		if result[i][0] != 1.0 {
			t.Errorf("result[%d][0]: want 1.0 (primary), got %v", i, result[i][0])
		}
	}
}

// -- Test 17: embed_chunks_per_call records value=1 for non-chunked calls --

// TestClient_EmbedWithResult_RecordsChunksMetricForNonChunked verifies that
// the embed_chunks_per_call histogram fires with value=1 for calls under
// the chunking threshold. Before the MAJOR 3 fix, this metric only fired
// for chunked calls, undercounting backend-call multipliers in dashboards.
func TestClient_EmbedWithResult_RecordsChunksMetricForNonChunked(t *testing.T) {
	var callCount atomic.Int32
	srv := chunkingTestServer(t, &callCount, nil)

	model := "non-chunk-metric-model"
	c, err := NewClient(srv.URL,
		WithModel(model),
		WithDim(1),
		WithChunkSize(32),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	histBefore := histogramSnapshot(embedChunksPerCall, model)

	// 16 texts < chunkSize=32 → no chunking, but metric MUST still record value=1.
	texts := make([]string, 16)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}
	if _, err := c.Embed(context.Background(), texts); err != nil {
		t.Fatalf("Embed: %v", err)
	}

	histAfter := histogramSnapshot(embedChunksPerCall, model)
	countDelta := histAfter.count - histBefore.count
	sumDelta := histAfter.sum - histBefore.sum

	if countDelta != 1 {
		t.Errorf("embed_chunks_per_call sample count delta: want 1, got %d", countDelta)
	}
	if sumDelta != 1 {
		t.Errorf("embed_chunks_per_call sum delta: want 1 (single call, no chunking), got %g", sumDelta)
	}
}

type histSnapshot struct {
	count uint64
	sum   float64
}

// histogramSnapshot reads the current sample count and sum for a given label
// from a HistogramVec. WithLabelValues returns prometheus.Observer; we type-assert
// to prometheus.Histogram (the concrete type) to access Write.
func histogramSnapshot(hv *prometheus.HistogramVec, labelVal string) histSnapshot {
	obs := hv.WithLabelValues(labelVal)
	h, ok := obs.(prometheus.Histogram)
	if !ok {
		return histSnapshot{}
	}
	var m dto.Metric
	_ = h.Write(&m)
	if m.Histogram == nil {
		return histSnapshot{}
	}
	return histSnapshot{
		count: m.Histogram.GetSampleCount(),
		sum:   m.Histogram.GetSampleSum(),
	}
}
