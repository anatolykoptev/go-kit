package middleware

import (
	"context"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// ShadowBan silently drops updates from chats where check returns true.
// No error is returned and no answer is sent — the handler is invisible to the abuser.
func ShadowBan(check func(chatID int64) bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			if check(chatIDFromUpdate(upd)) {
				return nil // silent drop
			}
			return next(ctx, upd)
		}
	}
}
