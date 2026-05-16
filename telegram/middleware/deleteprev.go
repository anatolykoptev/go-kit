package middleware

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// DeletePrev deletes the inline-keyboard source message BEFORE calling next().
// Best-effort: deletion failure is logged and ignored so next() always runs.
// Only fires when upd.CallbackQuery != nil and its Message is non-nil.
//
// Encapsulates the "delete source before reply" pattern from oxpulse-admin.
func DeletePrev(del func(chatID int64, messageID int) error) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			if upd.CallbackQuery == nil || upd.CallbackQuery.Message == nil {
				return next(ctx, upd)
			}
			cq := upd.CallbackQuery
			chatID := cq.Message.Chat.ID
			msgID := cq.Message.MessageID
			if err := del(chatID, msgID); err != nil {
				slog.WarnContext(ctx, "deleteprev: delete failed",
					"chat_id", chatID,
					"message_id", msgID,
					"err", err,
				)
			}
			return next(ctx, upd)
		}
	}
}
