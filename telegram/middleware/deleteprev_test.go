package middleware

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// TestDeletePrev_DeletesBeforeNext verifies Delete is called before next().
func TestDeletePrev_DeletesBeforeNext(t *testing.T) {
	type call struct {
		chatID int64
		msgID  int
	}
	var calls []call
	del := func(chatID int64, msgID int) error {
		calls = append(calls, call{chatID, msgID})
		return nil
	}
	h := DeletePrev(del)(nopHandler)
	upd := mkCbUpd(42, "cb", 100)
	if err := h(context.Background(), upd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("Delete called %d times, want 1", len(calls))
	}
	if calls[0].chatID != 42 || calls[0].msgID != 100 {
		t.Errorf("Delete(%v,%v), want (42,100)", calls[0].chatID, calls[0].msgID)
	}
}

// TestDeletePrev_DeleteFails_StillCallsNext verifies that Delete failure is tolerated.
func TestDeletePrev_DeleteFails_StillCallsNext(t *testing.T) {
	del := func(chatID int64, msgID int) error { return errors.New("msg too old") }
	called := false
	h := DeletePrev(del)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkCbUpd(1, "cb", 5)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() not called after Delete failure")
	}
}

// TestDeletePrev_NotForMessageEvent verifies DeletePrev skips Delete for Message events.
func TestDeletePrev_NotForMessageEvent(t *testing.T) {
	var delCalled bool
	del := func(chatID int64, msgID int) error { delCalled = true; return nil }
	called := false
	h := DeletePrev(del)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delCalled {
		t.Error("Delete called for message event")
	}
	if !called {
		t.Error("next() not called for message event")
	}
}
