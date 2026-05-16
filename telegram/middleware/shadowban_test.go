package middleware

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestShadowBan_BannedChat_SilentDrop verifies that banned chats are silently dropped.
func TestShadowBan_BannedChat_SilentDrop(t *testing.T) {
	banned := map[int64]bool{42: true}
	check := func(chatID int64) bool { return banned[chatID] }
	called := false
	h := ShadowBan(check)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkMsgUpd(42)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("next() called for banned chat, should be silently dropped")
	}
}

// TestShadowBan_UnbannedChat_PassesThrough verifies that non-banned chats pass through.
func TestShadowBan_UnbannedChat_PassesThrough(t *testing.T) {
	check := func(chatID int64) bool { return false }
	called := false
	h := ShadowBan(check)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkMsgUpd(99)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() not called for non-banned chat")
	}
}
