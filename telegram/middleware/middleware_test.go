package middleware

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// mkMsgUpd returns a minimal message Update for a given chatID.
func mkMsgUpd(chatID int64) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: tgbotapi.Chat{ID: chatID},
			From: &tgbotapi.User{ID: chatID},
		},
	}
}

// mkCbUpd returns a minimal callback-query Update.
func mkCbUpd(chatID int64, callbackID string, msgID int) *tgbotapi.Update {
	return &tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   callbackID,
			From: &tgbotapi.User{ID: chatID},
			Message: &tgbotapi.Message{
				MessageID: msgID,
				Chat:      tgbotapi.Chat{ID: chatID},
			},
		},
	}
}

// errAlways is a reusable handler that always returns an error.
var errAlways = func(ctx context.Context, upd *tgbotapi.Update) error {
	return errors.New("handler error")
}

// nopHandler is a reusable handler that does nothing.
var nopHandler = func(ctx context.Context, upd *tgbotapi.Update) error { return nil }

// TestChain_OrdersCorrectly verifies that Chain composes middlewares left-to-right
// (outermost first), so the first middleware in the list wraps the outermost call.
func TestChain_OrdersCorrectly(t *testing.T) {
	var order []string

	mw1 := func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			order = append(order, "mw1-before")
			err := next(ctx, upd)
			order = append(order, "mw1-after")
			return err
		}
	}
	mw2 := func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			order = append(order, "mw2-before")
			err := next(ctx, upd)
			order = append(order, "mw2-after")
			return err
		}
	}
	inner := func(ctx context.Context, upd *tgbotapi.Update) error {
		order = append(order, "inner")
		return nil
	}

	h := Chain(mw1, mw2)(inner)
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"mw1-before", "mw2-before", "inner", "mw2-after", "mw1-after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

// TestChain_EmptyHandler_Identity verifies that Chain with no middlewares is identity.
func TestChain_EmptyHandler_Identity(t *testing.T) {
	called := false
	inner := func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	}
	h := Chain()(inner)
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("inner handler was not called")
	}
}

// TestChain_NestedComposition_PreservesOrder verifies that Chain(Chain(a,b), c) ≡ Chain(a,b,c).
func TestChain_NestedComposition_PreservesOrder(t *testing.T) {
	var flat, nested []string

	makeRecord := func(name string, dst *[]string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, upd *tgbotapi.Update) error {
				*dst = append(*dst, name+"-before")
				err := next(ctx, upd)
				*dst = append(*dst, name+"-after")
				return err
			}
		}
	}
	inner := func(ctx context.Context, upd *tgbotapi.Update) error { return nil }

	// Flat: Chain(a, b, c)
	Chain(makeRecord("a", &flat), makeRecord("b", &flat), makeRecord("c", &flat))(inner)(context.Background(), mkMsgUpd(1)) //nolint:errcheck

	// Nested: Chain(Chain(a, b), c)
	Chain(Chain(makeRecord("a", &nested), makeRecord("b", &nested)), makeRecord("c", &nested))(inner)(context.Background(), mkMsgUpd(1)) //nolint:errcheck

	if len(flat) != len(nested) {
		t.Fatalf("flat=%v, nested=%v", flat, nested)
	}
	for i := range flat {
		if flat[i] != nested[i] {
			t.Errorf("index %d: flat=%q nested=%q", i, flat[i], nested[i])
		}
	}
}
