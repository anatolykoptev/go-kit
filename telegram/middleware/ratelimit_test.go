package middleware

import (
	"context"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/ratelimit"
)

// TestRateLimit_BelowBucket_Allows verifies that events within the token budget pass through.
func TestRateLimit_BelowBucket_Allows(t *testing.T) {
	kl := ratelimit.NewKeyLimiter(3, 3)
	defer kl.Close()

	count := 0
	h := RateLimit(kl, ByChatID, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		count++
		return nil
	})
	upd := mkMsgUpd(1)
	for i := range 3 {
		if err := h(context.Background(), upd); err != nil {
			t.Fatalf("event %d: unexpected error: %v", i, err)
		}
	}
	if count != 3 {
		t.Errorf("handler called %d times, want 3", count)
	}
}

// TestRateLimit_OverBucket_Drops verifies that events over the token budget are throttled.
func TestRateLimit_OverBucket_Drops(t *testing.T) {
	kl := ratelimit.NewKeyLimiter(3, 3)
	defer kl.Close()

	count := 0
	h := RateLimit(kl, ByChatID, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		count++
		return nil
	})
	upd := mkMsgUpd(1)
	for range 5 {
		h(context.Background(), upd) //nolint:errcheck
	}
	if count != 3 {
		t.Errorf("handler called %d times, want 3 (burst)", count)
	}
}

// TestRateLimit_OnDeny_Called verifies onDeny is invoked when throttled.
func TestRateLimit_OnDeny_Called(t *testing.T) {
	kl := ratelimit.NewKeyLimiter(1, 1)
	defer kl.Close()

	denied := 0
	onDeny := func(ctx context.Context, upd *tgbotapi.Update) error {
		denied++
		return nil
	}
	h := RateLimit(kl, ByChatID, onDeny)(nopHandler)
	upd := mkMsgUpd(1)
	for range 3 {
		h(context.Background(), upd) //nolint:errcheck
	}
	if denied == 0 {
		t.Error("onDeny never called when throttled")
	}
}

// TestRateLimit_PerChatKey verifies that different chat IDs have independent buckets.
func TestRateLimit_PerChatKey(t *testing.T) {
	kl := ratelimit.NewKeyLimiter(1, 1)
	defer kl.Close()

	count := 0
	h := RateLimit(kl, ByChatID, nil)(func(ctx context.Context, upd *tgbotapi.Update) error {
		count++
		return nil
	})
	h(context.Background(), mkMsgUpd(1)) //nolint:errcheck
	h(context.Background(), mkMsgUpd(2)) //nolint:errcheck
	if count != 2 {
		t.Errorf("handler called %d times, want 2 (independent per-chat buckets)", count)
	}
}

// TestByChatID_CallbackQuery verifies ByChatID extracts callback from.id.
func TestByChatID_CallbackQuery(t *testing.T) {
	upd := mkCbUpd(42, "cb", 0)
	if got := ByChatID(upd); got != "42" {
		t.Errorf("ByChatID = %q, want \"42\"", got)
	}
}

// TestByUserID_Message verifies ByUserID extracts Message.From.ID.
func TestByUserID_Message(t *testing.T) {
	upd := mkMsgUpd(99)
	if got := ByUserID(upd); got != "99" {
		t.Errorf("ByUserID = %q, want \"99\"", got)
	}
}
