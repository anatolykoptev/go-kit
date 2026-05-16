package middleware_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/telegram/middleware"
)

// makeUpdate returns an *tgbotapi.Update with a given chat ID.
func makeUpdate(chatID int64) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: chatID},
		},
	}
}

// noopHandler accepts the update and returns nil.
var noopHandler middleware.Handler = func(_ context.Context, _ *tgbotapi.Update) error {
	return nil
}

// TestNewChatQuota_AllowsKnownChats verifies that a chat already seen within the window
// passes through even when the new-chat count is at the limit.
func TestNewChatQuota_AllowsKnownChats(t *testing.T) {
	const limit = 3
	window := time.Hour

	mw := middleware.NewChatQuota(window, limit, middleware.ByChatID)
	handler := mw(noopHandler)

	ctx := context.Background()

	// Exhaust quota: 3 distinct new chats.
	for i := int64(1); i <= int64(limit); i++ {
		if err := handler(ctx, makeUpdate(i)); err != nil {
			t.Fatalf("chat %d should pass (new, within limit): %v", i, err)
		}
	}

	// A chat that was already seen must pass even though quota is full.
	for i := int64(1); i <= int64(limit); i++ {
		if err := handler(ctx, makeUpdate(i)); err != nil {
			t.Fatalf("known chat %d should pass (seen): %v", i, err)
		}
	}
}

// TestNewChatQuota_DropsExcessNewChats verifies that the (limit+1)-th distinct new chat
// in the window is dropped (handler not called, ErrNewChatQuotaExceeded returned).
func TestNewChatQuota_DropsExcessNewChats(t *testing.T) {
	const limit = 3
	window := time.Hour

	mw := middleware.NewChatQuota(window, limit, middleware.ByChatID)

	callCount := 0
	countingHandler := middleware.Handler(func(_ context.Context, _ *tgbotapi.Update) error {
		callCount++
		return nil
	})
	handler := mw(countingHandler)

	ctx := context.Background()

	// Fill to limit.
	for i := int64(1); i <= int64(limit); i++ {
		if err := handler(ctx, makeUpdate(i)); err != nil {
			t.Fatalf("chat %d should pass: %v", i, err)
		}
	}

	// (limit+1)-th new chat must be dropped.
	err := handler(ctx, makeUpdate(int64(limit+1)))
	if err == nil {
		t.Fatal("expected error for excess new chat, got nil")
	}
	if err != middleware.ErrNewChatQuotaExceeded {
		t.Fatalf("expected ErrNewChatQuotaExceeded, got: %v", err)
	}

	// handler must not have been called for the dropped update.
	if callCount != limit {
		t.Fatalf("handler called %d times, want %d", callCount, limit)
	}
}

// TestNewChatQuota_GCsExpiredEntries verifies that entries older than window
// are cleared on the next invocation, freeing quota.
func TestNewChatQuota_GCsExpiredEntries(t *testing.T) {
	// Very short window so we can advance past it without sleeping long.
	window := 50 * time.Millisecond
	const limit = 2

	mw := middleware.NewChatQuota(window, limit, middleware.ByChatID)
	handler := mw(noopHandler)

	ctx := context.Background()

	// Fill quota with 2 new chats.
	if err := handler(ctx, makeUpdate(1)); err != nil {
		t.Fatalf("chat 1 should pass: %v", err)
	}
	if err := handler(ctx, makeUpdate(2)); err != nil {
		t.Fatalf("chat 2 should pass: %v", err)
	}

	// Quota full — new chat 3 must be rejected.
	if err := handler(ctx, makeUpdate(3)); err != middleware.ErrNewChatQuotaExceeded {
		t.Fatalf("expected ErrNewChatQuotaExceeded before window expires, got: %v", err)
	}

	// Let window expire.
	time.Sleep(window + 10*time.Millisecond)

	// After GC (triggered by next call), old entries removed; chat 3 is now "new" and fits.
	if err := handler(ctx, makeUpdate(3)); err != nil {
		t.Fatalf("chat 3 should pass after window expiry: %v", err)
	}
}

// TestNewChatQuota_ConcurrentAccess_NoRace exercises concurrent access under the race detector.
// Run with: go test -race ./telegram/middleware/...
func TestNewChatQuota_ConcurrentAccess_NoRace(t *testing.T) {
	const limit = 100
	window := time.Hour

	mw := middleware.NewChatQuota(window, limit, middleware.ByChatID)
	handler := mw(noopHandler)

	ctx := context.Background()
	done := make(chan struct{})

	// 20 goroutines each sending 50 updates for different chat IDs.
	for g := 0; g < 20; g++ {
		g := g
		go func() {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 50; i++ {
				chatID := int64(g*1000 + i)
				// Ignore error — some will be dropped once quota fills.
				_ = handler(ctx, makeUpdate(chatID))
			}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// TestNewChatQuota_CustomKeyFn verifies that a caller-supplied keyFn is respected.
func TestNewChatQuota_CustomKeyFn(t *testing.T) {
	const limit = 2
	window := time.Hour

	// Key on user ID instead of chat ID.
	byUserID := func(upd *tgbotapi.Update) string {
		if upd.Message != nil && upd.Message.From != nil {
			return fmt.Sprintf("%d", upd.Message.From.ID)
		}
		return "unknown"
	}

	mw := middleware.NewChatQuota(window, limit, byUserID)
	handler := mw(noopHandler)

	ctx := context.Background()

	makeUpdWithUser := func(chatID, userID int64) *tgbotapi.Update {
		return &tgbotapi.Update{
			Message: &tgbotapi.Message{
				Chat: &tgbotapi.Chat{ID: chatID},
				From: &tgbotapi.User{ID: userID},
			},
		}
	}

	// Two distinct user IDs — pass.
	if err := handler(ctx, makeUpdWithUser(1, 100)); err != nil {
		t.Fatalf("user 100 should pass: %v", err)
	}
	if err := handler(ctx, makeUpdWithUser(2, 200)); err != nil {
		t.Fatalf("user 200 should pass: %v", err)
	}

	// Third distinct user ID — must be dropped.
	if err := handler(ctx, makeUpdWithUser(3, 300)); err != middleware.ErrNewChatQuotaExceeded {
		t.Fatalf("expected ErrNewChatQuotaExceeded for user 300, got: %v", err)
	}
}
