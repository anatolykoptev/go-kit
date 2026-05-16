package middleware

import (
	"context"
	"log/slog"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// AutoRespond calls answer(callbackID) BEFORE next(), regardless of next()'s outcome,
// including panics. This ensures the Telegram spinner always clears.
// Only fires when upd.CallbackQuery != nil; other update kinds pass through unchanged.
//
// If answer() returns an error, a warning is logged via slog but next() is still called —
// answer failure is best-effort (the spinner may not clear, but the handler must proceed).
//
// Concept lifted from telebot/v4/middleware/middleware.go:AutoRespond (MIT).
// Adapted: tele.Context → *tgbotapi.Update + injected answer func.
func AutoRespond(answer func(callbackID string) error) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			if upd.CallbackQuery == nil {
				return next(ctx, upd)
			}
			// Answer before next() so the spinner clears even if next() panics or errors.
			if err := answer(upd.CallbackQuery.ID); err != nil {
				slog.WarnContext(ctx, "AutoRespond: answer callback failed", "callback_id", upd.CallbackQuery.ID, "err", err)
			}
			return next(ctx, upd)
		}
	}
}
