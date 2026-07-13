package middleware

import (
	"context"
	"errors"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracingTracerName = "github.com/anatolykoptev/go-kit/telegram/middleware"

// Tracing returns a [Middleware] that opens an OpenTelemetry span around each
// Telegram update and records the result. Place it in the chain right after
// Recover (so panics are caught before the span is ended) and before Metrics
// (so tracing carries the same lifecycle context as metrics):
//
//	mw.Chain(
//	    Recover(log),
//	    Tracing("my-bot"),
//	    Metrics(reg, "my-bot_updates"),
//	    ...
//	)
//
// # Span name
//
//	"tg.update message"   — Message or EditedMessage
//	"tg.update callback"  — CallbackQuery
//	"tg.update other"     — any other update type (inline, channel post, etc.)
//
// # Attributes (always present)
//
//	rpc.system            = "telegram"
//	telegram.update.type  = "message" | "callback" | "other"
//
// # Attributes (when non-zero)
//
//	telegram.chat.id      = chat or user ID extracted from the update
//
// # Post-call attributes
//
//	telegram.status       = "ok" | "error" | "throttled"
//
// # Errors
//
//   - A nil error leaves span.Status at codes.Unset (success is the default).
//   - ErrThrottled sets codes.Error + telegram.status="throttled" without calling
//     RecordError — throttling is expected, not a bug.
//   - Any other non-nil error calls span.RecordError and sets codes.Error.
//
// scope is passed as the OTel instrumentation scope (service name). An empty
// scope falls back to the package tracer name.
func Tracing(scope string) Middleware {
	if scope == "" {
		scope = tracingTracerName
	}
	tracer := otel.Tracer(scope)

	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			updType := updateType(upd)
			spanName := "tg.update " + updType

			attrs := []attribute.KeyValue{
				attribute.String("rpc.system", "telegram"),
				attribute.String("telegram.update.type", updType),
			}
			if chatID := chatIDFromUpdate(upd); chatID != 0 {
				attrs = append(attrs, attribute.Int64("telegram.chat.id", chatID))
			}

			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			err := next(ctx, upd)

			switch {
			case err == nil:
				span.SetAttributes(attribute.String("telegram.status", "ok"))
				// codes.Unset is the OTel default for success; no SetStatus call needed.
			case errors.Is(err, ErrThrottled):
				span.SetAttributes(attribute.String("telegram.status", "throttled"))
				span.SetStatus(codes.Error, "throttled")
				// ErrThrottled is an expected flow-control outcome, not a bug;
				// RecordError is intentionally omitted to avoid noisy exception events.
			default:
				span.SetAttributes(attribute.String("telegram.status", "error"))
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}

			return err
		}
	}
}

// updateType classifies a Telegram Update into a short label suitable for
// use in span names and the telegram.update.type attribute.
//
// Labels match the Telegram Bot API field names shortened for readability:
//   - "message"  — Message (including replies, forwarded messages)
//   - "callback" — CallbackQuery (inline-keyboard button press)
//   - "other"    — anything else (inline queries, channel posts, etc.)
//
// EditedMessage is classified as "message" because the handling code is
// typically identical and keeping cardinality low is preferred.
func updateType(upd *tgbotapi.Update) string {
	if upd == nil {
		return "other"
	}
	switch {
	case upd.Message != nil || upd.EditedMessage != nil:
		return "message"
	case upd.CallbackQuery != nil:
		return "callback"
	default:
		return "other"
	}
}
