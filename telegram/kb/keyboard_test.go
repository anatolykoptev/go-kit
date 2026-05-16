package kb_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/telegram/kb"
)

// makeQuery builds a minimal CallbackQuery with the given data string.
func makeQuery(data string) *tgbotapi.CallbackQuery {
	return &tgbotapi.CallbackQuery{
		ID:   "qid",
		Data: data,
	}
}

// --- Test: WithPrefix option round-trips ---

func TestWithPrefix(t *testing.T) {
	k := kb.New(kb.WithPrefix("mypfx"))
	if got := k.Prefix(); got != "mypfx" {
		t.Fatalf("Prefix() = %q, want %q", got, "mypfx")
	}
}

func TestDefaultPrefixNonEmpty(t *testing.T) {
	k := kb.New()
	if p := k.Prefix(); p == "" {
		t.Fatal("default prefix must be non-empty")
	}
}

// --- Test: oversized data (>64 bytes) triggers onError at Button() ---

func TestOversizedDataButton(t *testing.T) {
	var capturedErr error
	k := kb.New(
		kb.WithPrefix("pfx"),
		kb.WithOnError(func(err error) { capturedErr = err }),
	)

	oversized := make([]byte, 65)
	for i := range oversized {
		oversized[i] = 'x'
	}
	k.Button("label", oversized, nil)

	if capturedErr == nil {
		t.Fatal("expected onError to be called for >64 byte data, got nil")
	}
	if !strings.Contains(capturedErr.Error(), "64") {
		t.Fatalf("error message should mention 64-byte limit, got: %v", capturedErr)
	}
}

// Exactly 64 bytes must NOT trigger error.
func TestExactly64BytesOK(t *testing.T) {
	var capturedErr error
	k := kb.New(
		kb.WithPrefix("pfx"),
		kb.WithOnError(func(err error) { capturedErr = err }),
	)
	data := make([]byte, 64)
	k.Button("label", data, nil)
	if capturedErr != nil {
		t.Fatalf("unexpected error for 64-byte data: %v", capturedErr)
	}
}

// --- Test: unknown prefix passthrough — Dispatch returns (false, nil) ---

func TestDispatchUnknownPrefix(t *testing.T) {
	k := kb.New(kb.WithPrefix("known"))
	k.Button("btn", []byte("d"), func(_ context.Context, _ *tgbotapi.CallbackQuery) error { return nil })

	cq := makeQuery("unknown0") // prefix "unknown" != "known"
	handled, err := k.Dispatch(context.Background(), cq)
	if handled {
		t.Fatal("Dispatch should return handled=false for unknown prefix")
	}
	if err != nil {
		t.Fatalf("Dispatch should return err=nil for unknown prefix, got: %v", err)
	}
}

// --- Test: WithDeleteOnClick flag round-trips via ShouldDeleteOnClick ---

func TestWithDeleteOnClick(t *testing.T) {
	k := kb.New(kb.WithDeleteOnClick(false))
	if k.ShouldDeleteOnClick() {
		t.Fatal("ShouldDeleteOnClick() should be false when WithDeleteOnClick(false) set")
	}

	k2 := kb.New(kb.WithDeleteOnClick(true))
	if !k2.ShouldDeleteOnClick() {
		t.Fatal("ShouldDeleteOnClick() should be true when WithDeleteOnClick(true) set")
	}
}

func TestDefaultDeleteOnClickIsTrue(t *testing.T) {
	k := kb.New()
	if !k.ShouldDeleteOnClick() {
		t.Fatal("default ShouldDeleteOnClick() should be true")
	}
}

// --- Test: Dispatch routes to the correct handler and returns (true, nil) ---

func TestDispatchKnownHandler(t *testing.T) {
	called := false
	k := kb.New(kb.WithPrefix("pfx"))
	k.Button("btn", []byte("d"), func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		called = true
		return nil
	})

	// Internal encoding: prefix + strconv.Itoa(index), first button = "pfx0"
	cq := makeQuery("pfx0")
	handled, err := k.Dispatch(context.Background(), cq)
	if !handled {
		t.Fatal("Dispatch should return handled=true for matching prefix")
	}
	if err != nil {
		t.Fatalf("Dispatch returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

// --- Test: handler error propagates through Dispatch ---

func TestDispatchHandlerError(t *testing.T) {
	sentinel := errors.New("handler error")
	k := kb.New(kb.WithPrefix("pfx"))
	k.Button("btn", []byte("d"), func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		return sentinel
	})

	cq := makeQuery("pfx0")
	handled, err := k.Dispatch(context.Background(), cq)
	if !handled {
		t.Fatal("should still be handled even if handler returns error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

// --- Test: Markup returns valid InlineKeyboardMarkup ---

func TestMarkup(t *testing.T) {
	k := kb.New(kb.WithPrefix("p")).
		Button("A", nil, nil).
		Row().
		Button("B", nil, nil)

	m := k.Markup()
	if len(m.InlineKeyboard) == 0 {
		t.Fatal("Markup should return at least one row")
	}
	// First row has button A, second row has button B
	if len(m.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(m.InlineKeyboard))
	}
	if m.InlineKeyboard[0][0].Text != "A" {
		t.Fatalf("expected first button to be 'A', got %q", m.InlineKeyboard[0][0].Text)
	}
	if m.InlineKeyboard[1][0].Text != "B" {
		t.Fatalf("expected second button to be 'B', got %q", m.InlineKeyboard[1][0].Text)
	}
}

// --- Test: URL button appears in Markup ---

func TestURLButton(t *testing.T) {
	k := kb.New(kb.WithPrefix("p")).
		URL("Visit", "https://example.com")

	m := k.Markup()
	if len(m.InlineKeyboard) == 0 || len(m.InlineKeyboard[0]) == 0 {
		t.Fatal("Markup should contain URL button")
	}
	btn := m.InlineKeyboard[0][0]
	if btn.Text != "Visit" {
		t.Fatalf("expected 'Visit', got %q", btn.Text)
	}
	if btn.URL == nil || *btn.URL != "https://example.com" {
		t.Fatalf("expected URL https://example.com, got %v", btn.URL)
	}
}

// TestDefaultOnError_UsesSlog verifies that the built-in error handler (no WithOnError)
// emits an slog record at ERROR or WARN level (not INFO as log.Printf would) (item 1.4 — v0.57 polish).
// We trigger it by passing data exceeding maxDataBytes (64) to Button().
func TestDefaultOnError_UsesSlog(t *testing.T) {
	var records []slog.Record
	handler := &kbCaptureHandler{records: &records}
	old := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(old) })

	k := kb.New() // uses defaultOnError
	oversized := strings.Repeat("x", 65)
	k.Button("label", []byte(oversized), func(_ context.Context, _ *tgbotapi.CallbackQuery) error {
		return nil
	})

	if len(records) != 1 {
		t.Fatalf("expected 1 slog record from defaultOnError, got %d", len(records))
	}
	// log.Printf routes through slog at INFO; explicit slog.Error/Warn is ERROR/WARN.
	if records[0].Level == slog.LevelInfo {
		t.Errorf("defaultOnError used log.Printf (level=INFO); want slog.Error or slog.Warn (level > INFO)")
	}
}

type kbCaptureHandler struct {
	records *[]slog.Record
}

func (h *kbCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *kbCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	*h.records = append(*h.records, r)
	return nil
}
func (h *kbCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *kbCaptureHandler) WithGroup(_ string) slog.Handler      { return h }
