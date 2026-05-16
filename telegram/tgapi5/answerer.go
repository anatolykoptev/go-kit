package tgapi5

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotCallbackAnswerer wraps *tgbotapi.BotAPI and satisfies middleware.CallbackAnswerer.
type BotCallbackAnswerer struct {
	bot *tgbotapi.BotAPI
}

// NewCallbackAnswerer returns a *BotCallbackAnswerer backed by bot.
func NewCallbackAnswerer(bot *tgbotapi.BotAPI) *BotCallbackAnswerer {
	return &BotCallbackAnswerer{bot: bot}
}

// AnswerCallback answers a Telegram callback query, clearing the spinner.
// Satisfies middleware.CallbackAnswerer.
func (a *BotCallbackAnswerer) AnswerCallback(callbackID string) error {
	_, err := a.bot.Request(tgbotapi.NewCallback(callbackID, ""))
	return err
}
