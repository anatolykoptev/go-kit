package middleware

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
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

// TestAutoRespond_AnswerFails_LogsWarn verifies that when answer() returns an error,
// AutoRespond logs a WarnContext entry (item 1.1 — v0.57 polish).
func TestAutoRespond_AnswerFails_LogsWarn(t *testing.T) {
	// Capture slog output by replacing the default handler temporarily.
	var records []slog.Record
	handler := &captureHandler{records: &records}
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(old) })

	answerErr := errors.New("tg: too many requests")
	answer := func(id string) error { return answerErr }
	h := AutoRespond(answer)(nopHandler)
	if err := h(context.Background(), mkCbUpd(1, "cb-warn", 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 slog record, got %d: %v", len(records), records)
	}
	if records[0].Level != slog.LevelWarn {
		t.Errorf("log level = %v, want Warn", records[0].Level)
	}
}

// captureHandler is a minimal slog.Handler that captures records for test assertions.
type captureHandler struct {
	records *[]slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }
