package middleware

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupTracingRecorder installs a fresh SpanRecorder as the global tracer
// and restores a no-op provider on cleanup. Returns the recorder.
func setupTracingRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(sdktrace.NewTracerProvider()) })
	return rec
}

// spanAttrValue returns the string value of a named attribute from a span,
// or "" if not found.
func spanAttrValue(s sdktrace.ReadOnlySpan, key string) string {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

// spanAttrInt64 returns the int64 value of a named attribute from a span,
// or 0 if not found.
func spanAttrInt64(s sdktrace.ReadOnlySpan, key string) int64 {
	for _, a := range s.Attributes() {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	return 0
}

// TestTracing_MessageUpdate verifies that a message update produces one span
// named "tg.update message", with telegram.update.type="message",
// telegram.chat.id set from the message, and status ok.
func TestTracing_MessageUpdate(t *testing.T) {
	rec := setupTracingRecorder(t)

	h := Tracing("test-svc")(nopHandler)
	if err := h(context.Background(), mkMsgUpd(42)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]

	if s.Name() != "tg.update message" {
		t.Errorf("span name = %q, want %q", s.Name(), "tg.update message")
	}
	if got := spanAttrValue(s, "telegram.update.type"); got != "message" {
		t.Errorf("telegram.update.type = %q, want %q", got, "message")
	}
	if got := spanAttrInt64(s, "telegram.chat.id"); got != 42 {
		t.Errorf("telegram.chat.id = %d, want 42", got)
	}
	if got := spanAttrValue(s, "rpc.system"); got != "telegram" {
		t.Errorf("rpc.system = %q, want %q", got, "telegram")
	}
	if got := spanAttrValue(s, "telegram.status"); got != "ok" {
		t.Errorf("telegram.status = %q, want %q", got, "ok")
	}
	if s.Status().Code != codes.Unset {
		t.Errorf("span status code = %v, want Unset for ok path", s.Status().Code)
	}
}

// TestTracing_CallbackUpdate verifies span name and chat id extraction
// from a callback-query update (From.ID, not Message.Chat.ID).
func TestTracing_CallbackUpdate(t *testing.T) {
	rec := setupTracingRecorder(t)

	h := Tracing("test-svc")(nopHandler)
	upd := mkCbUpd(99, "cb-001", 5)
	if err := h(context.Background(), upd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]

	if s.Name() != "tg.update callback" {
		t.Errorf("span name = %q, want %q", s.Name(), "tg.update callback")
	}
	if got := spanAttrValue(s, "telegram.update.type"); got != "callback" {
		t.Errorf("telegram.update.type = %q, want %q", got, "callback")
	}
	if got := spanAttrInt64(s, "telegram.chat.id"); got != 99 {
		t.Errorf("telegram.chat.id = %d, want 99", got)
	}
}

// TestTracing_OtherUpdate verifies that an update that is neither message
// nor callback produces the span name "tg.update other" and no chat_id attr.
func TestTracing_OtherUpdate(t *testing.T) {
	rec := setupTracingRecorder(t)

	h := Tracing("svc")(nopHandler)
	upd := &tgbotapi.Update{} // neither Message nor CallbackQuery
	if err := h(context.Background(), upd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "tg.update other" {
		t.Errorf("span name = %q, want %q", s.Name(), "tg.update other")
	}
	if got := spanAttrInt64(s, "telegram.chat.id"); got != 0 {
		// chat.id should be omitted when it is zero
		t.Errorf("telegram.chat.id = %d on 'other' update, want 0 (unset)", got)
	}
}

// TestTracing_ErrorHandler verifies that a handler error is recorded on the
// span: RecordError fires, StatusCode=Error, telegram.status=error.
func TestTracing_ErrorHandler(t *testing.T) {
	rec := setupTracingRecorder(t)

	sentinel := errors.New("handler boom")
	h := Tracing("svc")(func(ctx context.Context, upd *tgbotapi.Update) error {
		return sentinel
	})
	if err := h(context.Background(), mkMsgUpd(1)); !errors.Is(err, sentinel) {
		t.Fatalf("error not propagated; got %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]

	if s.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", s.Status().Code)
	}
	if got := spanAttrValue(s, "telegram.status"); got != "error" {
		t.Errorf("telegram.status = %q, want %q", got, "error")
	}
	// At least one exception event must be recorded.
	found := false
	for _, ev := range s.Events() {
		if ev.Name == "exception" {
			found = true
			break
		}
	}
	if !found {
		t.Error("no 'exception' event recorded on error span")
	}
}

// TestTracing_ErrThrottled verifies that ErrThrottled is treated as
// telegram.status=throttled and span status=Error.
func TestTracing_ErrThrottled(t *testing.T) {
	rec := setupTracingRecorder(t)

	h := Tracing("svc")(func(ctx context.Context, upd *tgbotapi.Update) error {
		return ErrThrottled
	})
	if err := h(context.Background(), mkMsgUpd(1)); !errors.Is(err, ErrThrottled) {
		t.Fatalf("ErrThrottled not propagated; got %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]

	if got := spanAttrValue(s, "telegram.status"); got != "throttled" {
		t.Errorf("telegram.status = %q, want %q", got, "throttled")
	}
	if s.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error for throttled", s.Status().Code)
	}
}

// TestTracing_EmptyScope verifies no panic when scope is ""; span is still emitted.
func TestTracing_EmptyScope(t *testing.T) {
	rec := setupTracingRecorder(t)

	h := Tracing("")(nopHandler)
	if err := h(context.Background(), mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rec.Ended()) != 1 {
		t.Fatalf("expected 1 span, got %d", len(rec.Ended()))
	}
}

// TestTracing_PropagatesParentContext verifies that a span created by Tracing
// is a child of any active span in the incoming context.
func TestTracing_PropagatesParentContext(t *testing.T) {
	rec := setupTracingRecorder(t)

	// Start a root span manually.
	ctx, root := otel.Tracer("test").Start(context.Background(), "root")
	defer root.End()

	h := Tracing("svc")(nopHandler)
	if err := h(ctx, mkMsgUpd(1)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// End the root span so it appears in rec.Ended() too.
	root.End()

	spans := rec.Ended()
	// Find the child span (name starts with "tg.update").
	var child sdktrace.ReadOnlySpan
	for _, s := range spans {
		if s.Name() != "root" {
			child = s
		}
	}
	if child == nil {
		t.Fatal("child span not found")
	}
	if child.Parent().TraceID() != root.SpanContext().TraceID() {
		t.Errorf("child traceID %v != root traceID %v", child.Parent().TraceID(), root.SpanContext().TraceID())
	}
	if child.Parent().SpanID() != root.SpanContext().SpanID() {
		t.Errorf("child parentSpanID %v != root spanID %v", child.Parent().SpanID(), root.SpanContext().SpanID())
	}
}
