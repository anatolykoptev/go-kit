package middleware

import (
	"context"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/ratelimit"
)

// TestFullChain_ComposesInDocumentedOrder verifies the recommended chain ordering:
//
//	Recover → Metrics → AutoRespond → DeletePrev → OperatorOnly → ShadowBan → RateLimit → handler
//
// Each middleware must fire in left-to-right (outer→inner) sequence.
//
// Historical note: modelled after "SurvivesContextRewrap" regression tests from
// oxpulse-admin (memory: feedback_regression_test_must_include_realistic_chain).
func TestFullChain_ComposesInDocumentedOrder(t *testing.T) {
	var order []string

	var answered []string
	answer := func(id string) error { answered = append(answered, id); return nil }
	var deleted []struct{ chatID int64; msgID int }
	del := func(chatID int64, msgID int) error {
		deleted = append(deleted, struct{ chatID int64; msgID int }{chatID, msgID})
		return nil
	}
	isOp := func(chatID int64) bool { return chatID == 1 }
	check := func(chatID int64) bool { return false } // nobody banned
	reg := metrics.NewRegistry()
	kl := ratelimit.NewKeyLimiter(10, 10)
	defer kl.Close()

	record := func(name string) Middleware {
		return func(next Handler) Handler {
			return func(ctx context.Context, upd *tgbotapi.Update) error {
				order = append(order, name+"-before")
				err := next(ctx, upd)
				order = append(order, name+"-after")
				return err
			}
		}
	}

	inner := func(ctx context.Context, upd *tgbotapi.Update) error {
		order = append(order, "handler")
		return nil
	}

	h := Chain(
		record("recover"),
		record("metrics"),
		record("autorespond"),
		record("deleteprev"),
		record("operatoronly"),
		record("shadowban"),
		record("ratelimit"),
	)(inner)

	// Also exercise the real middlewares in a separate chain.
	realH := Chain(
		Recover(func(v any) {}),
		Metrics(reg, "dispatch"),
		AutoRespond(answer),
		DeletePrev(del),
		OperatorOnly(isOp, nil),
		ShadowBan(check),
		RateLimit(kl, ByChatID, nil),
	)(nopHandler)

	upd := mkCbUpd(1, "cb-1", 7)

	if err := h(context.Background(), upd); err != nil {
		t.Fatalf("tracking chain: unexpected error: %v", err)
	}

	wantBefore := []string{"recover-before", "metrics-before", "autorespond-before",
		"deleteprev-before", "operatoronly-before", "shadowban-before", "ratelimit-before"}
	wantAfter := []string{"ratelimit-after", "shadowban-after", "operatoronly-after",
		"deleteprev-after", "autorespond-after", "metrics-after", "recover-after"}

	idx := 0
	for _, w := range wantBefore {
		if idx >= len(order) || order[idx] != w {
			t.Errorf("order[%d] = %q, want %q (full: %v)", idx, safeGet(order, idx), w, order)
		}
		idx++
	}
	if idx < len(order) && order[idx] != "handler" {
		t.Errorf("order[%d] = %q, want \"handler\"", idx, order[idx])
	}
	idx++
	for _, w := range wantAfter {
		if idx >= len(order) || order[idx] != w {
			t.Errorf("order[%d] = %q, want %q (full: %v)", idx, safeGet(order, idx), w, order)
		}
		idx++
	}

	// Run the real chain.
	if err := realH(context.Background(), upd); err != nil {
		t.Fatalf("real chain: unexpected error: %v", err)
	}
	if len(answered) == 0 {
		t.Error("AutoRespond did not call answer")
	}
	if len(deleted) == 0 {
		t.Error("DeletePrev did not call del")
	}
	if reg.Value("dispatch{result=ok}") != 1 {
		t.Errorf("dispatch ok counter = %d, want 1", reg.Value("dispatch{result=ok}"))
	}
}

func safeGet(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "<out of bounds>"
}

// TestFullChain_BannedChat_SilentDrop verifies that a banned chat is dropped with no error.
func TestFullChain_BannedChat_SilentDrop(t *testing.T) {
	reg := metrics.NewRegistry()
	kl := ratelimit.NewKeyLimiter(10, 10)
	defer kl.Close()

	called := false
	h := Chain(
		Recover(func(v any) {}),
		Metrics(reg, "dispatch"),
		AutoRespond(func(id string) error { return nil }),
		DeletePrev(func(chatID int64, msgID int) error { return nil }),
		OperatorOnly(func(chatID int64) bool { return chatID == 99 }, nil),
		ShadowBan(func(chatID int64) bool { return chatID == 99 }),
		RateLimit(kl, ByChatID, nil),
	)(func(ctx context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})

	upd := mkCbUpd(99, "cb-ban", 3)
	if err := h(context.Background(), upd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("inner handler called for banned chat")
	}
}
