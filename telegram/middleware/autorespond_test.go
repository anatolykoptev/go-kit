package middleware

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestAutoRespond_AlwaysCallsAnswer verifies Answer fires even when next() errors.
func TestAutoRespond_AlwaysCallsAnswer(t *testing.T) {
	var answered []string
	answer := func(id string) error { answered = append(answered, id); return nil }
	h := AutoRespond(answer)(errAlways)
	err := h(context.Background(), mkCbUpd(1, "cb-id-1", 0))
	if err == nil {
		t.Error("expected error from inner handler, got nil")
	}
	if len(answered) != 1 || answered[0] != "cb-id-1" {
		t.Errorf("Answer calls = %v, want [cb-id-1]", answered)
	}
}

// TestAutoRespond_NotForMessageEvent verifies AutoRespond skips Answer for Message events.
func TestAutoRespond_NotForMessageEvent(t *testing.T) {
	var answered []string
	answer := func(id string) error { answered = append(answered, id); return nil }
	h := AutoRespond(answer)(nopHandler)
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(answered) != 0 {
		t.Errorf("Answer was called for message event: calls=%v", answered)
	}
}

// TestAutoRespond_AnswerFails_StillCallsNext verifies that Answer failure doesn't block next().
func TestAutoRespond_AnswerFails_StillCallsNext(t *testing.T) {
	answer := func(id string) error { return errors.New("telegram error") }
	called := false
	h := AutoRespond(answer)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkCbUpd(1, "cb", 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() was not called after Answer failure")
	}
}
