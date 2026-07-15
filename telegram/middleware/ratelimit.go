package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/ratelimit"
)

// ErrThrottled is returned by RateLimit when the per-key token bucket is exhausted.
// Metrics inspects this sentinel to classify the result as "throttled".
var ErrThrottled = errors.New("rate limit exceeded")

// RateLimit wraps go-kit/ratelimit.KeyLimiter for per-key throttling.
// keyFn extracts the bucket key from the update (e.g. ByChatID, ByUserID).
// When the bucket is exhausted, onDeny is called instead of next (pass nil for silent drop).
func RateLimit(kl *ratelimit.KeyLimiter, keyFn func(*tgbotapi.Update) string, onDeny Handler) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			key := keyFn(upd)
			if !kl.Allow(key) {
				slog.DebugContext(ctx, "ratelimit: throttled", "key", key)
				if onDeny != nil {
					return onDeny(ctx, upd)
				}
				return ErrThrottled
			}
			return next(ctx, upd)
		}
	}
}

// ByChatID is a keyFn for RateLimit that buckets by chat/user ID.
// Suitable for per-chat rate limiting.
func ByChatID(upd *tgbotapi.Update) string {
	return fmt.Sprintf("%d", chatIDFromUpdate(upd))
}

// ByUserID is a keyFn for RateLimit that buckets by the acting user's ID.
// For callback queries this is the user who clicked; for messages it is Message.From.ID.
func ByUserID(upd *tgbotapi.Update) string {
	if upd.CallbackQuery != nil {
		return fmt.Sprintf("%d", upd.CallbackQuery.From.ID)
	}
	if upd.Message != nil && upd.Message.From != nil {
		return fmt.Sprintf("%d", upd.Message.From.ID)
	}
	return "unknown"
}
