package tgapi5

import (
	"errors"
	"strings"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/files"
)

var (
	// ErrNilBot is returned by [NewFileDownloader] for a nil bot.
	ErrNilBot = errors.New("tgapi5: nil bot")
	// ErrFileEndpoint is returned when the bot's file endpoint is not the
	// standard <base>/file/bot%s/%s shape, so the base URL cannot be derived.
	ErrFileEndpoint = errors.New("tgapi5: bot file endpoint has an unexpected shape")
)

// NewFileDownloader returns a [files.Downloader] configured like the bot: its
// token, its API/file endpoint (a self-hosted Bot API server) and its HTTP
// client. Download voice notes, photos and documents by file_id without ever
// building the token-bearing file URL; never call bot.GetFileDirectURL,
// bot.FileURL or File.Link, whose result carries the token.
//
// opts apply after the bot's settings, so [files.WithBaseURL] or
// [files.WithHTTPClient] override them. A nil bot returns [ErrNilBot].
func NewFileDownloader(bot *tgbotapi.BotAPI, opts ...files.Option) (*files.Downloader, error) {
	if bot == nil {
		return nil, ErrNilBot
	}
	base, err := fileBase(bot)
	if err != nil {
		return nil, err
	}
	all := []files.Option{files.WithBaseURL(base)}
	if bot.Client != nil {
		all = append(all, files.WithHTTPClient(bot.Client))
	}
	return files.New(bot.Token, append(all, opts...)...), nil
}

// fileBase derives the server base URL from the bot's file endpoint format. The
// SDK keeps the endpoint private; FileURL renders it, and the token it embeds
// is stripped here before anything leaves this function.
func fileBase(bot *tgbotapi.BotAPI) (string, error) {
	const probe = "p"
	u := bot.FileURL(tgbotapi.File{FilePath: probe})
	suffix := "/file/bot" + bot.Token + "/" + probe
	if bot.Token == "" || !strings.HasSuffix(u, suffix) {
		return "", ErrFileEndpoint
	}
	return strings.TrimSuffix(u, suffix), nil
}
