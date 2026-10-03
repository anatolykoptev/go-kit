package tgapi5

import (
	"log/slog"
	"net/http"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/tgsafe"
)

// DefaultSafeTimeout is the HTTP timeout of a bot built by [NewSafeBotAPI]:
// above the 60 s long-poll Telegram clients conventionally use, so a stalled
// connection cannot hang a poll forever.
const DefaultSafeTimeout = 75 * time.Second

// SafeOption configures [NewSafeBotAPI].
type SafeOption func(*safeConfig)

type safeConfig struct {
	base     *http.Client
	endpoint string
}

// WithBaseClient supplies the underlying HTTP client (transport, proxy, a
// different timeout). The default is &http.Client{Timeout: DefaultSafeTimeout}.
func WithBaseClient(c *http.Client) SafeOption {
	return func(sc *safeConfig) { sc.base = c }
}

// NewSafeBotAPI is tgbotapi.NewBotAPI with a client that scrubs the bot token
// out of every error ([tgsafe.NewHTTPClient]) and a default timeout. Every
// error the SDK returns for a failed API call is a *url.Error whose text
// carries bot<TOKEN>; with this constructor it carries bot<redacted> and keeps
// its type, so Timeout() and errors.As still work.
//
// Pair it with [SetSafeLogger]: GetUpdatesChan prints failed polls through the
// SDK's package logger.
func NewSafeBotAPI(token string, opts ...SafeOption) (*tgbotapi.BotAPI, error) {
	return newSafeBotAPI(token, tgbotapi.APIEndpoint, opts...)
}

func newSafeBotAPI(token, endpoint string, opts ...SafeOption) (*tgbotapi.BotAPI, error) {
	sc := safeConfig{endpoint: endpoint}
	for _, o := range opts {
		o(&sc)
	}
	if sc.base == nil {
		sc.base = &http.Client{Timeout: DefaultSafeTimeout}
	}
	return tgbotapi.NewBotAPIWithClient(token, sc.endpoint, tgsafe.NewHTTPClient(sc.base, token))
}

// SetSafeLogger replaces the SDK's package logger (stderr by default) with one
// that scrubs the given secrets and the bot segment and writes to l (nil means
// slog.Default()). The SDK's logger is process-global; call it once at startup.
func SetSafeLogger(l *slog.Logger, secrets ...string) error {
	return tgbotapi.SetLogger(tgsafe.NewLogger(l, secrets...))
}
