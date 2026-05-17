package broadcast_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/broadcast"
)

// fakeSender records all Chattable dispatches for assertion.
type fakeSender struct {
	mu      sync.Mutex
	sent    []tgbotapi.Chattable
	errFn   func(call int) error // if non-nil, called per dispatch
	callIdx int
}

func (f *fakeSender) SendChattable(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, c)
	idx := f.callIdx
	f.callIdx++
	if f.errFn != nil {
		return tgbotapi.Message{}, f.errFn(idx)
	}
	return tgbotapi.Message{}, nil
}

func (f *fakeSender) dispatches() []tgbotapi.Chattable {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tgbotapi.Chattable, len(f.sent))
	copy(out, f.sent)
	return out
}

// makeIDs returns a sequential slice of message IDs [1, n].
func makeIDs(n int) []int {
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i + 1
	}
	return ids
}

// noopPacer returns a 0-interval pacer (no delay) with no-op send fn.
func noopPacer() *broadcast.Pacer {
	return broadcast.NewPacer(func(_ context.Context, _ int64, _ string) error { return nil },
		broadcast.WithRPS(1000)) // high rate = effectively no delay
}

// ─── Copy ────────────────────────────────────────────────────────────────────

// TestBroadcaster_Copy_SingleChunk verifies copy with ≤100 IDs produces one
// CopyMessagesConfig per target.
func TestBroadcaster_Copy_SingleChunk(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	targets := []int64{10, 20, 30}
	opts := broadcast.BatchCopyOptions{
		FromChatID: 999,
		MessageIDs: makeIDs(5),
		ToChatIDs:  targets,
	}
	results := b.Copy(context.Background(), opts)

	// 3 targets × 1 chunk = 3 dispatches.
	dispatches := fs.dispatches()
	if len(dispatches) != 3 {
		t.Fatalf("expected 3 dispatches, got %d", len(dispatches))
	}

	for i, d := range dispatches {
		cfg, ok := d.(tgbotapi.CopyMessagesConfig)
		if !ok {
			t.Fatalf("dispatch[%d] is %T, want CopyMessagesConfig", i, d)
		}
		if cfg.FromChat.ChatID != 999 {
			t.Errorf("dispatch[%d].FromChat.ChatID=%d want 999", i, cfg.FromChat.ChatID)
		}
		if len(cfg.MessageIDs) != 5 {
			t.Errorf("dispatch[%d].MessageIDs len=%d want 5", i, len(cfg.MessageIDs))
		}
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("ChatID %d: unexpected error: %v", r.ChatID, r.Err)
		}
		if !r.AllOK() {
			t.Errorf("ChatID %d: AllOK() false unexpectedly", r.ChatID)
		}
	}
}

// TestBroadcaster_Copy_ChunksAt100 verifies 250 IDs produce 3 chunks per target.
func TestBroadcaster_Copy_ChunksAt100(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	targets := []int64{10, 20}
	opts := broadcast.BatchCopyOptions{
		FromChatID: 999,
		MessageIDs: makeIDs(250),
		ToChatIDs:  targets,
	}
	results := b.Copy(context.Background(), opts)

	// 2 targets × 3 chunks = 6 dispatches.
	dispatches := fs.dispatches()
	if len(dispatches) != 6 {
		t.Fatalf("expected 6 dispatches (2 targets × 3 chunks), got %d", len(dispatches))
	}

	expectedLens := []int{100, 100, 50}
	for i, expLen := range expectedLens {
		cfg, ok := dispatches[i].(tgbotapi.CopyMessagesConfig)
		if !ok {
			t.Fatalf("dispatch[%d] is %T, want CopyMessagesConfig", i, dispatches[i])
		}
		if len(cfg.MessageIDs) != expLen {
			t.Errorf("dispatch[%d].MessageIDs len=%d want %d", i, len(cfg.MessageIDs), expLen)
		}
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

// TestBroadcaster_Copy_FlagPlumbing verifies DisableNotification, ProtectContent,
// RemoveCaption are passed through to every CopyMessagesConfig.
func TestBroadcaster_Copy_FlagPlumbing(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	opts := broadcast.BatchCopyOptions{
		FromChatID:          999,
		MessageIDs:          makeIDs(3),
		ToChatIDs:           []int64{10},
		DisableNotification: true,
		ProtectContent:      true,
		RemoveCaption:       true,
	}
	b.Copy(context.Background(), opts)

	dispatches := fs.dispatches()
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	cfg := dispatches[0].(tgbotapi.CopyMessagesConfig)
	if !cfg.DisableNotification {
		t.Error("DisableNotification not plumbed through")
	}
	if !cfg.ProtectContent {
		t.Error("ProtectContent not plumbed through")
	}
	if !cfg.RemoveCaption {
		t.Error("RemoveCaption not plumbed through")
	}
}

// TestBroadcaster_Copy_ResultChatIDs verifies each TargetResult has the expected ChatID.
func TestBroadcaster_Copy_ResultChatIDs(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	targets := []int64{10, 20, 30}
	opts := broadcast.BatchCopyOptions{
		FromChatID: 999,
		MessageIDs: makeIDs(1),
		ToChatIDs:  targets,
	}
	results := b.Copy(context.Background(), opts)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	seen := make(map[int64]bool)
	for _, r := range results {
		seen[r.ChatID] = true
	}
	for _, id := range targets {
		if !seen[id] {
			t.Errorf("result missing ChatID %d", id)
		}
	}
}

// TestBroadcaster_Copy_CtxCancel verifies context cancellation is honoured.
func TestBroadcaster_Copy_CtxCancel(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	opts := broadcast.BatchCopyOptions{
		FromChatID: 999,
		MessageIDs: makeIDs(5),
		ToChatIDs:  []int64{10, 20, 30, 40, 50},
	}
	results := b.Copy(ctx, opts)
	_ = results // just ensure no panic
}

// TestBroadcaster_Copy_PacerRateLimit verifies that with a low-rate Pacer,
// the wall time for N targets is at least (N-1)/RPS.
func TestBroadcaster_Copy_PacerRateLimit(t *testing.T) {
	fs := &fakeSender{}
	// 2 RPS → 500ms between targets. 5 targets → at least 4 × 500ms = 2s.
	p := broadcast.NewPacer(func(_ context.Context, _ int64, _ string) error { return nil },
		broadcast.WithRPS(2))
	b := broadcast.NewBroadcaster(fs, p)

	targets := []int64{1, 2, 3, 4, 5}
	opts := broadcast.BatchCopyOptions{
		FromChatID: 1,
		MessageIDs: makeIDs(1),
		ToChatIDs:  targets,
	}

	start := time.Now()
	results := b.Copy(context.Background(), opts)
	elapsed := time.Since(start)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	// 5 targets, 2 RPS → minimum (5-1)×500ms = 2s wall time.
	minWall := 4 * (time.Second / 2)
	if elapsed < minWall {
		t.Errorf("wall time %v < expected minimum %v for 5 targets at 2 RPS", elapsed, minWall)
	}
}

// TestBroadcaster_Copy_ChunkErrs_PartialFailure verifies that per-chunk errors
// are captured in ChunkErrs and that AnyOK/AllOK report correctly.
func TestBroadcaster_Copy_ChunkErrs_PartialFailure(t *testing.T) {
	terminalErr := errors.New("terminal: chat not found")
	// Dispatch 0 fails (chunk 1), dispatch 1 succeeds (chunk 2).
	fs := &fakeSender{
		errFn: func(call int) error {
			if call == 0 {
				return terminalErr
			}
			return nil
		},
	}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	// 2 chunks: 1 msg each (use 2 msg IDs, size 1 per chunk for simplicity).
	// We need >100 IDs to get 2 chunks — or use internal chunk size. Instead,
	// use 101 IDs to get [100, 1] chunks.
	opts := broadcast.BatchCopyOptions{
		FromChatID: 1,
		MessageIDs: makeIDs(101),
		ToChatIDs:  []int64{42},
	}
	results := b.Copy(context.Background(), opts)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if len(r.ChunkErrs) != 2 {
		t.Fatalf("ChunkErrs len=%d; want 2", len(r.ChunkErrs))
	}
	if !errors.Is(r.ChunkErrs[0], terminalErr) {
		t.Errorf("ChunkErrs[0]=%v; want %v", r.ChunkErrs[0], terminalErr)
	}
	if r.ChunkErrs[1] != nil {
		t.Errorf("ChunkErrs[1]=%v; want nil", r.ChunkErrs[1])
	}
	if !errors.Is(r.Err, terminalErr) {
		t.Errorf("Err=%v; want %v", r.Err, terminalErr)
	}
	// AnyOK = true because chunk 2 succeeded.
	if !r.AnyOK() {
		t.Error("AnyOK() = false; want true (chunk 2 succeeded)")
	}
	// AllOK = false because chunk 1 failed.
	if r.AllOK() {
		t.Error("AllOK() = true; want false (chunk 1 failed)")
	}
}

// TestBroadcaster_Copy_AllChunksFail verifies AnyOK=false when all chunks fail.
func TestBroadcaster_Copy_AllChunksFail(t *testing.T) {
	terminalErr := errors.New("terminal: blocked")
	fs := &fakeSender{
		errFn: func(_ int) error { return terminalErr },
	}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	opts := broadcast.BatchCopyOptions{
		FromChatID: 1,
		MessageIDs: makeIDs(3),
		ToChatIDs:  []int64{42},
	}
	results := b.Copy(context.Background(), opts)
	r := results[0]

	if r.AnyOK() {
		t.Error("AnyOK() = true; want false (all chunks failed)")
	}
	if r.AllOK() {
		t.Error("AllOK() = true; want false")
	}
}
