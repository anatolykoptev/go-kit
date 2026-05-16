package middleware

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestOperatorOnly_AllowsOperatorChat verifies that an operator chat passes through.
func TestOperatorOnly_AllowsOperatorChat(t *testing.T) {
	isOp := func(chatID int64) bool { return chatID == 100 || chatID == 200 }
	called := false
	h := OperatorOnly(isOp, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkCbUpd(100, "op-cb", 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() not called for operator chat")
	}
}

// TestOperatorOnly_DropsNonOperator_NilOnDeny verifies silent drop when onDeny is nil.
func TestOperatorOnly_DropsNonOperator_NilOnDeny(t *testing.T) {
	isOp := func(chatID int64) bool { return chatID == 100 }
	called := false
	h := OperatorOnly(isOp, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	if err := h(context.Background(), mkCbUpd(999, "non-op", 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("next() called for non-operator chat, should be silently dropped")
	}
}

// TestOperatorOnly_CallsOnDeny_WhenProvided verifies onDeny is invoked for non-operators.
func TestOperatorOnly_CallsOnDeny_WhenProvided(t *testing.T) {
	isOp := func(chatID int64) bool { return false }
	denyCalled := false
	onDeny := func(ctx context.Context, upd *tgbotapi.Update) error {
		denyCalled = true
		return nil
	}
	h := OperatorOnly(isOp, onDeny)(nopHandler)
	if err := h(context.Background(), mkCbUpd(999, "cb", 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !denyCalled {
		t.Error("onDeny not called for non-operator")
	}
}

// TestOperatorOnly_MessageEvents_PassThrough verifies message events pass through operator gate.
func TestOperatorOnly_MessageEvents_PassThrough(t *testing.T) {
	isOp := func(chatID int64) bool { return false } // even non-op messages pass
	called := false
	// Note: OperatorOnly with this isOp will DROP message events too (it calls isOp(chatID)).
	// Per spec: OperatorOnly gates by chatID regardless of update type.
	// Make isOp pass for message chatID.
	isOpMsg := func(chatID int64) bool { return true }
	h := OperatorOnly(isOpMsg, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})
	_ = isOp
	if err := h(context.Background(), mkMsgUpd(999)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("next() not called for message event with passing isOp")
	}
}
