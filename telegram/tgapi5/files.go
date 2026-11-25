package tgapi5

import (
	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/files"
)

// NewFileDownloader returns a [files.Downloader] for the bot's token: download
// voice notes, photos and documents by file_id without ever building the
// token-bearing file URL. Never call bot.GetFileDirectURL or File.Link: their
// result carries the token.
//
// The downloader gets its own HTTP client (set one with [files.WithHTTPClient]),
// not the bot's, so the bot's timeout and long-poll settings do not apply.
func NewFileDownloader(bot *tgbotapi.BotAPI, opts ...files.Option) *files.Downloader {
	return files.New(bot.Token, opts...)
}
