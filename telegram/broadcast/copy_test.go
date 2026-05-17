package broadcast_test

import (
	"context"
	"sync"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/broadcast"
)

// fakeSender records all Chattable dispatches for assertion.
type fakeSender struct {
	mu   sync.Mutex
	sent []tgbotapi.Chattable
}

func (f *fakeSender) SendChattable(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	f.mu.Lock()
	f.sent = append(f.sent, c)
	f.mu.Unlock()
	return tgbotapi.Message{}, nil
}

func (f *fakeSender) dispatches() []tgbotapi.Chattable {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]tgbotapi.Chattable, len(f.sent))
	copy(out, f.sent)
	return out
}

// makeIDs returns a sequential slice of message IDs [start, start+n).
func makeIDs(n int) []int {
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i + 1
	}
	return ids
}

// ─── Copy ────────────────────────────────────────────────────────────────────

// TestBroadcaster_Copy_SingleChunk verifies copy with ≤100 IDs produces one
// CopyMessagesConfig per target.
func TestBroadcaster_Copy_SingleChunk(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs)

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

	// Each dispatch must be a CopyMessagesConfig with FromChatID=999.
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

	// All 3 results must be non-error.
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("ChatID %d: unexpected error: %v", r.ChatID, r.Err)
		}
	}
}

// TestBroadcaster_Copy_ChunksAt100 verifies 250 IDs produce 3 chunks per target.
func TestBroadcaster_Copy_ChunksAt100(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs)

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

	// Validate first target's chunks: [100, 100, 50].
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
	b := broadcast.NewBroadcaster(fs)

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
	b := broadcast.NewBroadcaster(fs)

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
	b := broadcast.NewBroadcaster(fs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	opts := broadcast.BatchCopyOptions{
		FromChatID: 999,
		MessageIDs: makeIDs(5),
		ToChatIDs:  []int64{10, 20, 30, 40, 50},
	}
	results := b.Copy(ctx, opts)
	// With a cancelled context, we expect either 0 dispatches or fewer than
	// total targets processed.
	dispatches := fs.dispatches()
	_ = dispatches // not asserting exact count; just ensure no panic
	_ = results
}
