package notify_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/telegram/notify"
)

// ---------------------------------------------------------------------------
// Stub BotSender
// ---------------------------------------------------------------------------

// stubSender is a test double for notify.BotSender.
type stubSender struct {
	// sendHTML is called when ParseMode == "HTML"; returns htmlErr.
	htmlErr error
	// plainErr is returned on the fallback plain send (ParseMode == "").
	plainErr error
	// htmlCalls counts SendChattable calls where ParseMode == HTML.
	htmlCalls atomic.Int32
	// plainCalls counts SendChattable calls where ParseMode == "" (plain fallback).
	plainCalls atomic.Int32
}

func (s *stubSender) Send(chatID int64, text string) error { return nil }

func (s *stubSender) SendChattable(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, ok := c.(tgbotapi.MessageConfig)
	if !ok {
		return tgbotapi.Message{}, errors.New("stubSender: unexpected Chattable type")
	}
	if msg.ParseMode == tgbotapi.ModeHTML {
		s.htmlCalls.Add(1)
		return tgbotapi.Message{}, s.htmlErr
	}
	// Plain fallback (no ParseMode).
	s.plainCalls.Add(1)
	return tgbotapi.Message{}, s.plainErr
}

// ---------------------------------------------------------------------------
// Paced fan-out
// ---------------------------------------------------------------------------

// TestProductSink_NotifyPacedFanOut verifies all ChatIDs receive the message
// and the sent counter matches.
// Red-on-revert: remove the Pacer.Broadcast wiring and this test fails
// (send count would be 0).
func TestProductSink_NotifyPacedFanOut(t *testing.T) {
	stub := &stubSender{}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub,
		notify.WithRPS(1000),
		notify.WithProductMetrics(m),
	)

	p := notify.Product{
		Text:    "Hello <b>world</b>",
		ChatIDs: []int64{1, 2, 3},
	}
	sent, failed, err := sink.Notify(context.Background(), p)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent=%d, want 3", sent)
	}
	if failed != 0 {
		t.Errorf("failed=%d, want 0", failed)
	}
	if stub.htmlCalls.Load() != 3 {
		t.Errorf("htmlCalls=%d, want 3", stub.htmlCalls.Load())
	}
}

// TestProductSink_HTMLFallbackOnParseError verifies that when HTML send fails
// the sink retries in plain-text mode (go-hully bot.go:283 pattern).
// Red-on-revert: remove the fallback branch in buildSendFn and this test fails
// (plainCalls stays 0, error is returned as terminal).
func TestProductSink_HTMLFallbackOnParseError(t *testing.T) {
	stub := &stubSender{
		htmlErr:  errors.New("Bad Request: can't parse entities"),
		plainErr: nil,
	}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub,
		notify.WithRPS(1000),
		notify.WithHTMLFallback(true),
		notify.WithProductMetrics(m),
	)

	sent, failed, err := sink.Notify(context.Background(), notify.Product{
		Text:    "<b>Important</b>",
		ChatIDs: []int64{42},
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent=%d, want 1", sent)
	}
	if failed != 0 {
		t.Errorf("failed=%d, want 0 (plain fallback succeeded)", failed)
	}
	if stub.plainCalls.Load() != 1 {
		t.Errorf("plainCalls=%d, want 1 (fallback should have been tried)", stub.plainCalls.Load())
	}
}

// TestProductSink_HTMLFallbackDisabled verifies that WithHTMLFallback(false)
// does NOT attempt plain-text and counts the send as failed.
func TestProductSink_HTMLFallbackDisabled(t *testing.T) {
	stub := &stubSender{
		htmlErr:  errors.New("Bad Request: can't parse entities"),
		plainErr: nil,
	}
	sink := notify.NewProductSink(stub,
		notify.WithRPS(1000),
		notify.WithHTMLFallback(false),
	)

	sent, failed, _ := sink.Notify(context.Background(), notify.Product{
		Text:    "<b>msg</b>",
		ChatIDs: []int64{7},
	})
	if sent != 0 {
		t.Errorf("sent=%d, want 0 (no fallback, HTML failed)", sent)
	}
	if failed != 1 {
		t.Errorf("failed=%d, want 1", failed)
	}
	if stub.plainCalls.Load() != 0 {
		t.Errorf("plainCalls=%d, want 0 (fallback disabled)", stub.plainCalls.Load())
	}
}

// ---------------------------------------------------------------------------
// Result counters
// ---------------------------------------------------------------------------

// TestProductSink_SentCounterIncremented verifies notify_product_total{result=sent}
// is bumped for every successful delivery.
// Red-on-revert: remove the Add("notify_product_total{result=sent}", ...) call
// and this test fails.
func TestProductSink_SentCounterIncremented(t *testing.T) {
	stub := &stubSender{}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub, notify.WithRPS(1000), notify.WithProductMetrics(m))

	_, _, _ = sink.Notify(context.Background(), notify.Product{
		Text:    "hi",
		ChatIDs: []int64{1, 2},
	})

	key := metrics.Label(notify.MetricProductTotal, "result", "sent")
	if got := m.Value(key); got != 2 {
		t.Errorf("sent counter=%d, want 2", got)
	}
}

// TestProductSink_FailedCounterIncremented verifies notify_product_total{result=failed}
// is bumped for every terminal send failure.
// Red-on-revert: remove the metric bump in the DLQ callback and this fails.
func TestProductSink_FailedCounterIncremented(t *testing.T) {
	stub := &stubSender{
		htmlErr:  errors.New("Forbidden: bot was blocked by the user"),
		plainErr: errors.New("Forbidden: bot was blocked by the user"),
	}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub, notify.WithRPS(1000), notify.WithProductMetrics(m))

	_, failed, _ := sink.Notify(context.Background(), notify.Product{
		Text:    "blocked",
		ChatIDs: []int64{9, 10},
	})

	if failed != 2 {
		t.Errorf("failed=%d, want 2", failed)
	}
	key := metrics.Label(notify.MetricProductTotal, "result", "failed")
	// Each chat ID fails: each one tries HTML + fallback plain, both fail →
	// broadcast.Pacer calls DLQ once per terminal failure.
	if got := m.Value(key); got != 2 {
		t.Errorf("failed counter=%d, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// Dead-letter
// ---------------------------------------------------------------------------

// TestProductSink_DeadLetterCallback verifies that terminal failures are
// forwarded to the user-supplied dead-letter callback.
func TestProductSink_DeadLetterCallback(t *testing.T) {
	terminalErr := errors.New("Forbidden: bot was blocked by the user")
	stub := &stubSender{htmlErr: terminalErr, plainErr: terminalErr}

	var dlqIDs []int64
	sink := notify.NewProductSink(stub,
		notify.WithRPS(1000),
		notify.WithDeadLetter(func(chatID int64, err error) {
			dlqIDs = append(dlqIDs, chatID)
		}),
	)

	_, _, _ = sink.Notify(context.Background(), notify.Product{
		Text:    "msg",
		ChatIDs: []int64{5, 6},
	})

	if len(dlqIDs) != 2 {
		t.Errorf("DLQ received %d IDs, want 2", len(dlqIDs))
	}
}

// ---------------------------------------------------------------------------
// NotifyTo
// ---------------------------------------------------------------------------

// TestProductSink_NotifyTo_Success verifies single-recipient delivery.
func TestProductSink_NotifyTo_Success(t *testing.T) {
	stub := &stubSender{}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub, notify.WithProductMetrics(m))

	if err := sink.NotifyTo(context.Background(), 99, "direct ping"); err != nil {
		t.Fatalf("NotifyTo: %v", err)
	}
	key := metrics.Label(notify.MetricProductTotal, "result", "sent")
	if got := m.Value(key); got != 1 {
		t.Errorf("sent counter=%d, want 1", got)
	}
}

// TestProductSink_NotifyTo_Failure verifies error propagation and fail counter.
func TestProductSink_NotifyTo_Failure(t *testing.T) {
	stub := &stubSender{
		htmlErr:  errors.New("Forbidden: blocked"),
		plainErr: errors.New("Forbidden: blocked"),
	}
	m := metrics.NewRegistry()
	sink := notify.NewProductSink(stub, notify.WithProductMetrics(m))

	err := sink.NotifyTo(context.Background(), 99, "blocked")
	if err == nil {
		t.Fatal("NotifyTo: expected error, got nil")
	}
	key := metrics.Label(notify.MetricProductTotal, "result", "failed")
	if got := m.Value(key); got != 1 {
		t.Errorf("failed counter=%d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// Guard: empty ChatIDs
// ---------------------------------------------------------------------------

// TestProductSink_EmptyChatIDsReturnsError is a programming-mistake guard.
func TestProductSink_EmptyChatIDsReturnsError(t *testing.T) {
	stub := &stubSender{}
	sink := notify.NewProductSink(stub)
	_, _, err := sink.Notify(context.Background(), notify.Product{Text: "hi", ChatIDs: nil})
	if err == nil {
		t.Fatal("expected error for empty ChatIDs, got nil")
	}
}
