package middleware

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/metrics"
)

// TestMetrics_OkResult verifies that a successful handler increments the ok counter.
func TestMetrics_OkResult(t *testing.T) {
	reg := metrics.NewRegistry()
	h := Metrics(reg, "my_handler")(nopHandler)
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := reg.Snapshot()
	if snap["my_handler{result=ok}"] != 1 {
		t.Errorf("ok counter = %d, want 1; snapshot=%v", snap["my_handler{result=ok}"], snap)
	}
}

// TestMetrics_ErrorResult verifies that a failing handler increments the error counter.
func TestMetrics_ErrorResult(t *testing.T) {
	reg := metrics.NewRegistry()
	h := Metrics(reg, "my_handler")(errAlways)
	h(context.Background(), mkMsgUpd(1)) //nolint:errcheck
	snap := reg.Snapshot()
	if snap["my_handler{result=error}"] != 1 {
		t.Errorf("error counter = %d, want 1; snapshot=%v", snap["my_handler{result=error}"], snap)
	}
}

// TestMetrics_ThrottledResult verifies that ErrThrottled increments the throttled counter.
func TestMetrics_ThrottledResult(t *testing.T) {
	reg := metrics.NewRegistry()
	throttledHandler := func(ctx context.Context, upd *tgbotapi.Update) error {
		return ErrThrottled
	}
	h := Metrics(reg, "my_handler")(throttledHandler)
	h(context.Background(), mkMsgUpd(1)) //nolint:errcheck
	snap := reg.Snapshot()
	if snap["my_handler{result=throttled}"] != 1 {
		t.Errorf("throttled counter = %d, want 1; snapshot=%v", snap["my_handler{result=throttled}"], snap)
	}
}

// TestMetrics_MultipleInvocations verifies counters accumulate correctly.
func TestMetrics_MultipleInvocations(t *testing.T) {
	reg := metrics.NewRegistry()
	h := Metrics(reg, "h")(func(ctx context.Context, upd *tgbotapi.Update) error {
		chatID := int64(0)
		if upd.Message != nil {
			chatID = upd.Message.Chat.ID
		}
		if chatID == 1 {
			return nil
		}
		return errors.New("fail")
	})
	h(context.Background(), mkMsgUpd(1)) //nolint:errcheck
	h(context.Background(), mkMsgUpd(1)) //nolint:errcheck
	h(context.Background(), mkMsgUpd(2)) //nolint:errcheck

	snap := reg.Snapshot()
	if snap["h{result=ok}"] != 2 {
		t.Errorf("ok = %d, want 2", snap["h{result=ok}"])
	}
	if snap["h{result=error}"] != 1 {
		t.Errorf("error = %d, want 1", snap["h{result=error}"])
	}
}
