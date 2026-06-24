package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/env"
	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/telegram"
	"github.com/anatolykoptev/go-kit/telegram/broadcast"
	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// BotSender is the minimal interface productSink requires from a Telegram bot.
// *tgapi5.BotSender satisfies it; test doubles can substitute a stub.
type BotSender interface {
	// Send delivers plain text to chatID (satisfies middleware.Sender).
	Send(chatID int64, text string) error
	// SendChattable delivers an arbitrary Chattable (e.g. with parse mode / markup).
	SendChattable(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// MetricProductTotal is the counter bumped on each send attempt by ProductSink.
// Labels: result={sent,failed}.
const MetricProductTotal = "notify_product_total"

// defaultProductRPS mirrors broadcast.defaultRPS (30 — the Telegram Bot API
// global rate limit). Kept local because the broadcast const is unexported.
const defaultProductRPS = 30

// Product is one per-event product notification.
type Product struct {
	// Text is the already-formatted message body. The sink calls
	// telegram.PrepareForTelegram on it to detect HTML/Markdown/plain and
	// normalize to the Telegram HTML parse mode.
	Text string
	// ChatIDs is the list of recipient chat IDs. Must have at least one entry.
	ChatIDs []int64
}

// ProductSink delivers per-event product notifications to a bot, with
// bounded-rate fan-out (via broadcast.Pacer), retry, dead-letter, and an
// HTML→plaintext fallback on parse-mode failure (borrowed from go-hully
// bot.go:274–291).
type ProductSink interface {
	// Notify sends p to all ChatIDs in p. Returns the count of successful and
	// failed sends plus any context-level error that stopped the broadcast early.
	// Individual send failures are counted and dead-lettered but do not abort
	// the broadcast.
	Notify(ctx context.Context, p Product) (sent, failed int, err error)

	// NotifyTo sends text to a single chat ID, bypassing p.ChatIDs.
	// Useful for targeted sends where the recipient is determined at call time.
	NotifyTo(ctx context.Context, chatID int64, text string) error
}

// ProductOption configures a productSink.
type ProductOption func(*productSink)

// WithRPS sets the maximum sends per second for the underlying Pacer.
// Default: 30 (Telegram Bot API global rate limit).
func WithRPS(rps int) ProductOption {
	return func(s *productSink) { s.rps = rps }
}

// WithDeadLetter registers a callback invoked for each chat ID whose send
// failed terminally (non-transient, retries exhausted).
func WithDeadLetter(fn func(chatID int64, err error)) ProductOption {
	return func(s *productSink) { s.deadLetter = fn }
}

// WithProductMetrics wires a metrics registry.
// Bumps notify_product_total{result=sent} and notify_product_total{result=failed}.
func WithProductMetrics(m *metrics.Registry) ProductOption {
	return func(s *productSink) { s.m = m }
}

// WithHTMLFallback controls whether the sink tries plain-text on HTML parse
// failure. Default true (matches go-hully's production behaviour).
func WithHTMLFallback(enabled bool) ProductOption {
	return func(s *productSink) { s.htmlFallback = enabled }
}

// NewProductSink builds a ProductSink that uses sender for delivery.
// sender is typically *tgapi5.BotSender from tgapi5.NewSender, but any
// implementation of BotSender works.
func NewProductSink(sender BotSender, opts ...ProductOption) ProductSink {
	s := &productSink{
		sender:       sender,
		rps:          defaultProductRPS,
		htmlFallback: true,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// NewProductSinkFromEnv builds a ProductSink from environment variables.
//
// Required env var:
//   - TELEGRAM_BOT_TOKEN — bot token
//
// Optional env var:
//   - <prefix>_NOTIFY_CHAT_ID — default chat ID (parsed as int64 by ParseChatID)
//     e.g. BOUNTY_NOTIFY_CHAT_ID=428660
//
// m may be nil.
func NewProductSinkFromEnv(prefix string, m *metrics.Registry) (ProductSink, error) {
	token, err := env.Required("TELEGRAM_BOT_TOKEN")
	if err != nil {
		return nil, fmt.Errorf("notify: NewProductSinkFromEnv: %w", err)
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("notify: NewProductSinkFromEnv: create bot: %w", err)
	}
	sender := tgapi5.NewSender(bot, m)
	return NewProductSink(sender, WithProductMetrics(m)), nil
}

// productSink is the concrete ProductSink implementation.
type productSink struct {
	sender       BotSender
	rps          int
	deadLetter   func(chatID int64, err error)
	m            *metrics.Registry
	htmlFallback bool
}

// Notify implements ProductSink.
func (s *productSink) Notify(ctx context.Context, p Product) (sent, failed int, err error) {
	if len(p.ChatIDs) == 0 {
		return 0, 0, errors.New("notify: Product.ChatIDs must not be empty")
	}

	text, _ := telegram.PrepareForTelegram(p.Text)

	sendFn := s.buildSendFn(text)

	// Wrap in a dead-letter callback that also bumps the metric.
	dlq := func(chatID int64, dlErr error) {
		s.m.Incr(metrics.Label(MetricProductTotal, "result", "failed"))
		if s.deadLetter != nil {
			s.deadLetter(chatID, dlErr)
		}
	}

	pacer := broadcast.NewPacer(sendFn,
		broadcast.WithRPS(s.rps),
		broadcast.WithDeadLetter(dlq),
	)

	sent, failed, err = pacer.Broadcast(ctx, p.ChatIDs, text)

	// Count successes in the metric (Pacer only fires DLQ for failures).
	if sent > 0 {
		s.m.Add(metrics.Label(MetricProductTotal, "result", "sent"), int64(sent))
	}
	return sent, failed, err
}

// NotifyTo implements ProductSink.
func (s *productSink) NotifyTo(ctx context.Context, chatID int64, text string) error {
	prepared, _ := telegram.PrepareForTelegram(text)
	sendFn := s.buildSendFn(prepared)
	if err := sendFn(ctx, chatID, prepared); err != nil {
		s.m.Incr(metrics.Label(MetricProductTotal, "result", "failed"))
		return err
	}
	s.m.Incr(metrics.Label(MetricProductTotal, "result", "sent"))
	return nil
}

// buildSendFn returns a broadcast.SendFn that delivers HTML with a
// plaintext fallback (go-hully bot.go:274–291 pattern).
// htmlText is the pre-prepared HTML string passed to the closure.
func (s *productSink) buildSendFn(htmlText string) broadcast.SendFn {
	return func(ctx context.Context, chatID int64, _ string) error {
		// First attempt: HTML parse mode.
		msg := tgbotapi.NewMessage(chatID, htmlText)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.LinkPreviewOptions = tgbotapi.LinkPreviewOptions{IsDisabled: true}

		if _, err := s.sender.SendChattable(msg); err == nil {
			return nil
		} else if !s.htmlFallback {
			return err
		}

		// Fallback: strip HTML tags and send as plain text.
		plain := telegram.StripHTMLTags(htmlText)
		if plain == "" {
			plain = htmlText // last resort: send the raw string
		}
		slog.Warn("notify: HTML send failed, falling back to plain text",
			slog.Int64("chat_id", chatID))
		plain2 := tgbotapi.NewMessage(chatID, plain)
		plain2.LinkPreviewOptions = tgbotapi.LinkPreviewOptions{IsDisabled: true}
		_, err := s.sender.SendChattable(plain2)
		return err
	}
}

// Compile-time check: *tgapi5.BotSender satisfies the BotSender interface
// declared in this package.
var _ BotSender = (*tgapi5.BotSender)(nil)
