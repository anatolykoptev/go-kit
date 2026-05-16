package middleware

import (
	"context"
	"errors"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/metrics"
)

// Metrics auto-increments a counter labelled {result=ok|error|throttled}
// on every handler invocation. The counter name follows go-kit/metrics label syntax:
//
//	"<name>{result=ok}"
//	"<name>{result=error}"
//	"<name>{result=throttled}"
//
// result=throttled is triggered by the ErrThrottled sentinel (returned by RateLimit).
// All other non-nil errors produce result=error.
func Metrics(reg *metrics.Registry, name string) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			err := next(ctx, upd)
			result := classifyResult(err)
			reg.Incr(name + "{result=" + result + "}")
			return err
		}
	}
}

func classifyResult(err error) string {
	if err == nil {
		return "ok"
	}
	if errors.Is(err, ErrThrottled) {
		return "throttled"
	}
	return "error"
}
