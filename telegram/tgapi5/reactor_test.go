package tgapi5_test

import (
	"context"
	"testing"

	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// ─── Reactor tests ──────────────────────────────────────────────────────────

func TestReactor_SetReaction_Success(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 100, 42, []string{"👍", "❤️"}, false)
	if err != nil {
		t.Fatalf("SetReaction: unexpected error: %v", err)
	}
}

func TestReactor_SetReaction_IsBig(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	// IsBig=true should still succeed when server returns ok.
	err := r.SetReaction(context.Background(), 100, 42, []string{"🔥"}, true)
	if err != nil {
		t.Fatalf("SetReaction with IsBig: unexpected error: %v", err)
	}
}

func TestReactor_SetReaction_Error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 999, 42, []string{"👍"}, false)
	if err == nil {
		t.Fatal("SetReaction: expected error, got nil")
	}
}

func TestReactor_DeleteReaction_CallsSetReactionWithEmptySlice(t *testing.T) {
	// DeleteReaction is implemented as SetReaction with empty reaction slice.
	// The mock server returns ok for /setMessageReaction.
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.DeleteReaction(context.Background(), 100, 42)
	if err != nil {
		t.Fatalf("DeleteReaction: unexpected error: %v", err)
	}
}

func TestReactor_DeleteAllReactions_CallsSetReactionWithEmptySlice(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.DeleteAllReactions(context.Background(), 100, 42)
	if err != nil {
		t.Fatalf("DeleteAllReactions: unexpected error: %v", err)
	}
}

// TestReactor_SetReaction_EmptyEmojis verifies empty slice is accepted (no Telegram call error
// from our code; server decides).
func TestReactor_SetReaction_EmptyEmojis(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/setMessageReaction": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	r := tgapi5.NewReactor(bot)

	err := r.SetReaction(context.Background(), 100, 42, []string{}, false)
	if err != nil {
		t.Fatalf("SetReaction empty emojis: unexpected error: %v", err)
	}
}
