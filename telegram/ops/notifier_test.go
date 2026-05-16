package ops_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/telegram/ops"
)

// mockSender is a test implementation of ops.Sender.
type mockSender struct {
	calls   atomic.Int32
	lastMsg atomic.Value // stores string
	err     error
}

func (m *mockSender) Send(ctx context.Context, chatID int64, text string, buttons []ops.Button) error {
	m.calls.Add(1)
	m.lastMsg.Store(text)
	return m.err
}

// TestNotifier_SendDelivers verifies Send calls the underlying Sender.
//
// Spec §4.I: Send(ctx, req) → delivers message to operator chat.
func TestNotifier_SendDelivers(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(12345, sender)

	ctx := context.Background()
	req := ops.Request{
		Title: "Alert",
		Body:  "Something happened",
	}

	if err := n.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sender.calls.Load() != 1 {
		t.Fatalf("Sender.Send called %d times, want 1", sender.calls.Load())
	}
}

// TestNotifier_ButtonCallbackResolvesToHandler verifies HandleCallback dispatches
// to the correct button handler.
//
// Spec §4.I: HandleCallback looks up registered button handler by callback data,
// returns (handled, err).
func TestNotifier_ButtonCallbackResolvesToHandler(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(12345, sender)

	var handlerFired atomic.Bool
	var handlerData atomic.Value

	ctx := context.Background()
	req := ops.Request{
		Title: "Approval request",
		Body:  "Do you approve?",
		Buttons: []ops.Button{
			{
				Label:        "Approve",
				CallbackData: "approve:req42",
				Handler: func(ctx context.Context, data string) error {
					handlerFired.Store(true)
					handlerData.Store(data)
					return nil
				},
			},
			{
				Label:        "Deny",
				CallbackData: "deny:req42",
				Handler: func(ctx context.Context, data string) error {
					return nil
				},
			},
		},
	}

	if err := n.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Simulate incoming callback from Telegram.
	cq := &tgbotapi.CallbackQuery{
		Data: "approve:req42",
		From: &tgbotapi.User{ID: 99},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 12345},
		},
	}

	handled, err := n.HandleCallback(ctx, cq)
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}
	if !handled {
		t.Fatal("HandleCallback: want handled=true")
	}
	if !handlerFired.Load() {
		t.Fatal("Button handler not fired")
	}
	if storedData, _ := handlerData.Load().(string); storedData != "approve:req42" {
		t.Fatalf("Handler got data=%q, want %q", storedData, "approve:req42")
	}
}

// TestNotifier_HandleCallback_UnknownData returns (false, nil) for unknown data.
//
// Spec §4.I: returns (handled, err); unknown → not handled.
func TestNotifier_HandleCallback_UnknownData(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(12345, sender)

	cq := &tgbotapi.CallbackQuery{
		Data: "unknown:data",
		From: &tgbotapi.User{ID: 99},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 12345},
		},
	}

	handled, err := n.HandleCallback(context.Background(), cq)
	if err != nil {
		t.Fatalf("HandleCallback unknown: %v", err)
	}
	if handled {
		t.Fatal("HandleCallback unknown: want handled=false")
	}
}

// TestNotifier_CoalesceTriggersAboveThreshold verifies that when more than N
// sends fire within the window, individual sends are suppressed and a summary
// is sent instead.
//
// Spec §4.I: WithCoalesce(threshold, window) → "📥 N pending".
func TestNotifier_CoalesceTriggersAboveThreshold(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(
		12345,
		sender,
		ops.WithCoalesce(3, 500*time.Millisecond),
	)

	ctx := context.Background()
	req := ops.Request{Title: "alert", Body: "event"}

	// Send threshold+1 times to trigger coalescing.
	for i := 0; i < 5; i++ {
		_ = n.Send(ctx, req)
	}

	calls := int(sender.calls.Load())
	// With coalescing at threshold=3 over 5 sends: first 3 go through,
	// then burst triggers suppression → summary sent. Total calls < 5.
	// Exact count depends on implementation; the invariant is < 5.
	if calls >= 5 {
		t.Fatalf("coalescing: sender called %d times for 5 sends above threshold=3; expected fewer", calls)
	}
}

// TestNotifier_ProbeSuccess verifies probe result is prepended with ✅ on success.
//
// Spec §4.I: WithProbe(fn) → pre-Send pings URL, prepends ✅/⚠️.
func TestNotifier_ProbeSuccess(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(
		12345,
		sender,
		ops.WithProbe(func(ctx context.Context) (string, error) {
			return "HTTP 200 OK", nil
		}),
	)

	ctx := context.Background()
	req := ops.Request{Title: "Deploy", Body: "v1.2.0 deployed"}

	if err := n.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg, _ := sender.lastMsg.Load().(string)
	if len(msg) == 0 {
		t.Fatal("no message sent")
	}
	// Must contain ✅ prefix (probe succeeded).
	if !containsRune(msg, '✅') {
		t.Fatalf("probe success: message %q does not contain ✅", msg)
	}
}

// TestNotifier_ProbeFailure verifies probe error is prepended with ⚠️ on failure.
//
// Spec §4.I: pre-Send pings URL, prepends ✅/⚠️.
func TestNotifier_ProbeFailure(t *testing.T) {
	sender := &mockSender{}
	n := ops.NewNotifier(
		12345,
		sender,
		ops.WithProbe(func(ctx context.Context) (string, error) {
			return "", errors.New("connection refused")
		}),
	)

	ctx := context.Background()
	req := ops.Request{Title: "Deploy", Body: "v1.2.0 deployed"}

	if err := n.Send(ctx, req); err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg, _ := sender.lastMsg.Load().(string)
	if !containsRune(msg, '⚠') {
		t.Fatalf("probe failure: message %q does not contain ⚠️", msg)
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
