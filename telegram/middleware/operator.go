package middleware

import (
	"context"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// chatIDFromUpdate extracts the chat/user ID for operator and rate-limit checks.
// Prefers CallbackQuery.From.ID (the acting user), falls back to Message.Chat.ID.
func chatIDFromUpdate(upd *tgbotapi.Update) int64 {
	if upd.CallbackQuery != nil {
		return upd.CallbackQuery.From.ID
	}
	if upd.Message != nil {
		return upd.Message.Chat.ID
	}
	return 0
}

// OperatorOnly drops updates from non-operator chats by calling onDeny instead of next.
// isOp is called with the chat/user ID; if it returns false, onDeny is invoked.
// Pass nil onDeny for a silent drop.
//
// Spinner-clearing note: the recommended chain order places AutoRespond OUTSIDE
// OperatorOnly, so the spinner clears regardless of this gate's decision.
func OperatorOnly(isOp func(chatID int64) bool, onDeny Handler) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			if isOp(chatIDFromUpdate(upd)) {
				return next(ctx, upd)
			}
			if onDeny != nil {
				return onDeny(ctx, upd)
			}
			return nil
		}
	}
}
