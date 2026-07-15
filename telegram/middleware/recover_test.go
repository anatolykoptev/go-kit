package middleware

import (
	"context"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// TestRecover_CatchesPanic_ReturnsErr verifies that Recover catches a panic and returns a wrapped error.
func TestRecover_CatchesPanic_ReturnsErr(t *testing.T) {
	var logged []any
	logFn := func(v any) { logged = append(logged, v) }

	panicking := func(ctx context.Context, upd *tgbotapi.Update) error {
		panic("something went wrong")
	}
	h := Recover(logFn)(panicking)
	err := h(context.Background(), mkCbUpd(1, "cb-recover", 0))
	if err == nil {
		t.Error("expected error from Recover, got nil")
	}
	if len(logged) == 0 {
		t.Error("log function not called on panic")
	}
}

// TestRecover_NoPanic_PassesThrough verifies that Recover is transparent when next() doesn't panic.
func TestRecover_NoPanic_PassesThrough(t *testing.T) {
	var logged []any
	logFn := func(v any) { logged = append(logged, v) }
	called := false
	h := Recover(logFn)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() not called")
	}
	if len(logged) != 0 {
		t.Errorf("log called unexpectedly: %v", logged)
	}
}

// TestRecover_StringPanic_ReturnsError verifies that a string panic produces a non-nil error.
func TestRecover_StringPanic_ReturnsError(t *testing.T) {
	logFn := func(v any) {}
	h := Recover(logFn)(func(ctx context.Context, upd *tgbotapi.Update) error {
		panic("string panic")
	})
	err := h(context.Background(), mkMsgUpd(1))
	if err == nil {
		t.Error("expected non-nil error for string panic")
	}
}
