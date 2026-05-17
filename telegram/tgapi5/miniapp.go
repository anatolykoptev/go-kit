package tgapi5

import (
	"context"
	"encoding/json"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/miniapp"
)

// BotInvoiceSender adapts *tgbotapi.BotAPI to the miniapp.Sender interface.
// It supports SendInvoice (via bot.Send) and CreateInvoiceLink (via bot.Request),
// both honouring the caller's context via RequestWithContext.
type BotInvoiceSender struct {
	bot *tgbotapi.BotAPI
}

// NewInvoiceSender returns a BotInvoiceSender backed by bot.
// It satisfies miniapp.Sender.
func NewInvoiceSender(bot *tgbotapi.BotAPI) *BotInvoiceSender {
	return &BotInvoiceSender{bot: bot}
}

// SendInvoice sends a Telegram invoice using bot.RequestWithContext.
// Implements miniapp.Sender.
func (s *BotInvoiceSender) SendInvoice(ctx context.Context, cfg tgbotapi.InvoiceConfig) (*tgbotapi.Message, error) {
	resp, err := s.bot.RequestWithContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var msg tgbotapi.Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// CreateInvoiceLink creates a Telegram invoice link using bot.RequestWithContext.
// Implements miniapp.Sender.
func (s *BotInvoiceSender) CreateInvoiceLink(ctx context.Context, cfg tgbotapi.InvoiceLinkConfig) (string, error) {
	resp, err := s.bot.RequestWithContext(ctx, cfg)
	if err != nil {
		return "", err
	}
	var link string
	if err := json.Unmarshal(resp.Result, &link); err != nil {
		return "", err
	}
	return link, nil
}

// BotWebAppAnswerer adapts *tgbotapi.BotAPI to the miniapp.WebAppAnswerer interface.
type BotWebAppAnswerer struct {
	bot *tgbotapi.BotAPI
}

// NewWebAppAnswerer returns a BotWebAppAnswerer backed by bot.
// It satisfies miniapp.WebAppAnswerer.
func NewWebAppAnswerer(bot *tgbotapi.BotAPI) *BotWebAppAnswerer {
	return &BotWebAppAnswerer{bot: bot}
}

// AnswerWebAppQuery answers a Web App query identified by queryID.
// Implements miniapp.WebAppAnswerer.
func (a *BotWebAppAnswerer) AnswerWebAppQuery(
	ctx context.Context,
	queryID string,
	result tgbotapi.InlineQueryResult,
) (*tgbotapi.SentWebAppMessage, error) {
	cfg := tgbotapi.AnswerWebAppQueryConfig{
		WebAppQueryID: queryID,
		Result:        result,
	}
	resp, err := a.bot.RequestWithContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var msg tgbotapi.SentWebAppMessage
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// Compile-time interface satisfaction checks.
var (
	_ miniapp.Sender          = (*BotInvoiceSender)(nil)
	_ miniapp.WebAppAnswerer  = (*BotWebAppAnswerer)(nil)
)
