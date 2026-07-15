package broadcast_test

import (
	"context"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/broadcast"
)

// ─── Delete ──────────────────────────────────────────────────────────────────

// TestBroadcaster_Delete_ExactChunks verifies 250 IDs produce 3 DeleteMessagesConfig
// dispatches with [100, 100, 50] ID counts.
func TestBroadcaster_Delete_ExactChunks(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	err := b.Delete(context.Background(), 123, makeIDs(250))
	if err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	dispatches := fs.dispatches()
	if len(dispatches) != 3 {
		t.Fatalf("expected 3 dispatches, got %d", len(dispatches))
	}

	expectedLens := []int{100, 100, 50}
	for i, expLen := range expectedLens {
		cfg, ok := dispatches[i].(tgbotapi.DeleteMessagesConfig)
		if !ok {
			t.Fatalf("dispatch[%d] is %T, want DeleteMessagesConfig", i, dispatches[i])
		}
		if len(cfg.MessageIDs) != expLen {
			t.Errorf("dispatch[%d].MessageIDs len=%d want %d", i, len(cfg.MessageIDs), expLen)
		}
	}
}

// TestBroadcaster_Delete_ExactBoundary verifies exactly 100 IDs produce 1 dispatch.
func TestBroadcaster_Delete_ExactBoundary(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	err := b.Delete(context.Background(), 123, makeIDs(100))
	if err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	dispatches := fs.dispatches()
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	cfg := dispatches[0].(tgbotapi.DeleteMessagesConfig)
	if len(cfg.MessageIDs) != 100 {
		t.Errorf("MessageIDs len=%d want 100", len(cfg.MessageIDs))
	}
}

// TestBroadcaster_Delete_SingleMessage verifies 1 ID produces 1 dispatch.
func TestBroadcaster_Delete_SingleMessage(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	err := b.Delete(context.Background(), 456, []int{99})
	if err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	dispatches := fs.dispatches()
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 dispatch, got %d", len(dispatches))
	}
	cfg := dispatches[0].(tgbotapi.DeleteMessagesConfig)
	if cfg.ChatConfig.ChatID != 456 {
		t.Errorf("ChatID=%d want 456", cfg.ChatConfig.ChatID)
	}
	if len(cfg.MessageIDs) != 1 || cfg.MessageIDs[0] != 99 {
		t.Errorf("MessageIDs=%v want [99]", cfg.MessageIDs)
	}
}

// TestBroadcaster_Delete_ChunkIDOrder verifies IDs in chunks preserve original order.
func TestBroadcaster_Delete_ChunkIDOrder(t *testing.T) {
	fs := &fakeSender{}
	b := broadcast.NewBroadcaster(fs, noopPacer())

	ids := makeIDs(150) // [1..150]
	err := b.Delete(context.Background(), 123, ids)
	if err != nil {
		t.Fatalf("Delete: unexpected error: %v", err)
	}

	dispatches := fs.dispatches()
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 dispatches, got %d", len(dispatches))
	}

	// First chunk: IDs 1..100.
	chunk0 := dispatches[0].(tgbotapi.DeleteMessagesConfig).MessageIDs
	if chunk0[0] != 1 || chunk0[len(chunk0)-1] != 100 {
		t.Errorf("chunk0 range: [%d..%d] want [1..100]", chunk0[0], chunk0[len(chunk0)-1])
	}

	// Second chunk: IDs 101..150.
	chunk1 := dispatches[1].(tgbotapi.DeleteMessagesConfig).MessageIDs
	if chunk1[0] != 101 || chunk1[len(chunk1)-1] != 150 {
		t.Errorf("chunk1 range: [%d..%d] want [101..150]", chunk1[0], chunk1[len(chunk1)-1])
	}
}
