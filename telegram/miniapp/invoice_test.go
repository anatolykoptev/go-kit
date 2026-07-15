package miniapp

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// fakeSender implements Sender for testing.
// It records the last config passed and returns a fixed response or error.
type fakeSender struct {
	lastInvoiceCfg     tgbotapi.InvoiceConfig
	lastInvoiceLinkCfg tgbotapi.InvoiceLinkConfig
	invoiceCallCount   int
	linkCallCount      int
	returnMsg          *tgbotapi.Message
	returnLink         string
	returnErr          error
}

func (f *fakeSender) SendInvoice(
	_ context.Context,
	cfg tgbotapi.InvoiceConfig,
) (*tgbotapi.Message, error) {
	f.invoiceCallCount++
	f.lastInvoiceCfg = cfg
	return f.returnMsg, f.returnErr
}

func (f *fakeSender) CreateInvoiceLink(
	_ context.Context,
	cfg tgbotapi.InvoiceLinkConfig,
) (string, error) {
	f.linkCallCount++
	f.lastInvoiceLinkCfg = cfg
	return f.returnLink, f.returnErr
}

// baseValidParams returns a valid InvoiceParams set to exercise the golden path.
func baseValidParams() InvoiceParams {
	return InvoiceParams{
		ChatID:        123,
		Title:         "Widget",
		Description:   "A fine widget",
		Payload:       "p-1",
		ProviderToken: "tok-abc",
		Currency:      "USD",
		Prices:        []tgbotapi.LabeledPrice{{Label: "Widget", Amount: 100}},
	}
}

// baseLinkParams returns a valid InvoiceParams for CreateInvoiceLink (ChatID=0, MessageThread=0).
func baseLinkParams() InvoiceParams {
	p := baseValidParams()
	p.ChatID = 0
	p.MessageThread = 0
	return p
}

// --- SendInvoice tests ---

func TestSendInvoice_GoldenPath(t *testing.T) {
	wantMsg := &tgbotapi.Message{MessageID: 7}
	fs := &fakeSender{returnMsg: wantMsg}
	p := baseValidParams()

	got, err := SendInvoice(context.Background(), fs, p)
	if err != nil {
		t.Fatalf("SendInvoice golden: unexpected error: %v", err)
	}
	if got != wantMsg {
		t.Errorf("SendInvoice returned %+v; want %+v", got, wantMsg)
	}
	if fs.invoiceCallCount != 1 {
		t.Errorf("SendInvoice: Sender called %d times; want 1", fs.invoiceCallCount)
	}
	// Verify core fields plumbed into config.
	cfg := fs.lastInvoiceCfg
	if cfg.BaseChat.ChatConfig.ChatID != p.ChatID {
		t.Errorf("cfg.ChatID = %d; want %d", cfg.BaseChat.ChatConfig.ChatID, p.ChatID)
	}
	if cfg.Title != p.Title {
		t.Errorf("cfg.Title = %q; want %q", cfg.Title, p.Title)
	}
	if cfg.Currency != p.Currency {
		t.Errorf("cfg.Currency = %q; want %q", cfg.Currency, p.Currency)
	}
	if cfg.ProviderToken != p.ProviderToken {
		t.Errorf("cfg.ProviderToken = %q; want %q", cfg.ProviderToken, p.ProviderToken)
	}
}

func TestSendInvoice_MessageThread_PlumbedIntoConfig(t *testing.T) {
	fs := &fakeSender{returnMsg: &tgbotapi.Message{}}
	p := baseValidParams()
	p.MessageThread = 7

	_, err := SendInvoice(context.Background(), fs, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fs.lastInvoiceCfg.BaseChat.MessageThreadID != 7 {
		t.Errorf("MessageThreadID = %d; want 7", fs.lastInvoiceCfg.BaseChat.MessageThreadID)
	}
}

func TestSendInvoice_ErrInvalidTitle(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Title = ""

	_, err := SendInvoice(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidTitle) {
		t.Errorf("SendInvoice empty title: got %v; want ErrInvalidTitle", err)
	}
	if fs.invoiceCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestSendInvoice_ErrInvalidPrices(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Prices = nil

	_, err := SendInvoice(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidPrices) {
		t.Errorf("SendInvoice nil prices: got %v; want ErrInvalidPrices", err)
	}
	if fs.invoiceCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestSendInvoice_ErrInvalidStarsConfig(t *testing.T) {
	// XTR currency with non-empty ProviderToken must fail.
	fs := &fakeSender{}
	p := baseValidParams()
	p.Currency = "XTR"
	p.ProviderToken = "some-token"

	_, err := SendInvoice(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidStarsConfig) {
		t.Errorf("SendInvoice XTR+token: got %v; want ErrInvalidStarsConfig", err)
	}
	if fs.invoiceCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestSendInvoice_ErrInvalidProviderToken(t *testing.T) {
	// Non-XTR currency with empty ProviderToken must fail.
	fs := &fakeSender{}
	p := baseValidParams()
	p.Currency = "USD"
	p.ProviderToken = ""

	_, err := SendInvoice(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidProviderToken) {
		t.Errorf("SendInvoice USD+emptyToken: got %v; want ErrInvalidProviderToken", err)
	}
	if fs.invoiceCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestSendInvoice_XTRWithEmptyToken_GoldenPath(t *testing.T) {
	// XTR + empty token is the valid Stars path.
	fs := &fakeSender{returnMsg: &tgbotapi.Message{}}
	p := baseValidParams()
	p.Currency = "XTR"
	p.ProviderToken = ""

	_, err := SendInvoice(context.Background(), fs, p)
	if err != nil {
		t.Fatalf("XTR+empty token: unexpected error: %v", err)
	}
	if fs.invoiceCallCount != 1 {
		t.Errorf("Sender called %d times; want 1", fs.invoiceCallCount)
	}
}

// --- CreateInvoiceLink tests ---

func TestCreateInvoiceLink_GoldenPath(t *testing.T) {
	fs := &fakeSender{returnLink: "https://t.me/invoice/abc"}
	p := baseLinkParams()

	got, err := CreateInvoiceLink(context.Background(), fs, p)
	if err != nil {
		t.Fatalf("CreateInvoiceLink golden: unexpected error: %v", err)
	}
	if got != "https://t.me/invoice/abc" {
		t.Errorf("CreateInvoiceLink returned %q; want %q", got, "https://t.me/invoice/abc")
	}
	if fs.linkCallCount != 1 {
		t.Errorf("Sender called %d times; want 1", fs.linkCallCount)
	}
	// Verify core fields plumbed into config.
	cfg := fs.lastInvoiceLinkCfg
	if cfg.Title != p.Title {
		t.Errorf("cfg.Title = %q; want %q", cfg.Title, p.Title)
	}
	if cfg.Currency != p.Currency {
		t.Errorf("cfg.Currency = %q; want %q", cfg.Currency, p.Currency)
	}
	if cfg.ProviderToken != p.ProviderToken {
		t.Errorf("cfg.ProviderToken = %q; want %q", cfg.ProviderToken, p.ProviderToken)
	}
}

func TestCreateInvoiceLink_ErrInvalidTitle(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Title = ""

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidTitle) {
		t.Errorf("CreateInvoiceLink empty title: got %v; want ErrInvalidTitle", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestCreateInvoiceLink_ErrInvalidPrices(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Prices = nil

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidPrices) {
		t.Errorf("CreateInvoiceLink nil prices: got %v; want ErrInvalidPrices", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestCreateInvoiceLink_ErrInvalidStarsConfig(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Currency = "XTR"
	p.ProviderToken = "tok"

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidStarsConfig) {
		t.Errorf("CreateInvoiceLink XTR+token: got %v; want ErrInvalidStarsConfig", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestCreateInvoiceLink_ErrInvalidProviderToken(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.Currency = "EUR"
	p.ProviderToken = ""

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvalidProviderToken) {
		t.Errorf("CreateInvoiceLink EUR+emptyToken: got %v; want ErrInvalidProviderToken", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

// --- CreateInvoiceLink link-specific validation ---

func TestCreateInvoiceLink_ErrChatIDNotAllowed(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.ChatID = 123 // non-zero — must be rejected

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvoiceLinkChatIDNotAllowed) {
		t.Errorf("CreateInvoiceLink with ChatID!=0: got %v; want ErrInvoiceLinkChatIDNotAllowed", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestCreateInvoiceLink_ErrMessageThreadNotAllowed(t *testing.T) {
	fs := &fakeSender{}
	p := baseValidParams()
	p.ChatID = 0
	p.MessageThread = 7 // non-zero — must be rejected

	_, err := CreateInvoiceLink(context.Background(), fs, p)
	if !errors.Is(err, ErrInvoiceLinkMessageThreadNotAllowed) {
		t.Errorf("CreateInvoiceLink with MessageThread!=0: got %v; want ErrInvoiceLinkMessageThreadNotAllowed", err)
	}
	if fs.linkCallCount != 0 {
		t.Error("Sender must not be called on validation failure")
	}
}

func TestCreateInvoiceLink_ZeroChatIDAndThread_GoldenPath(t *testing.T) {
	fs := &fakeSender{returnLink: "https://t.me/invoice/ok"}
	p := baseValidParams()
	p.ChatID = 0
	p.MessageThread = 0

	got, err := CreateInvoiceLink(context.Background(), fs, p)
	if err != nil {
		t.Fatalf("CreateInvoiceLink zero fields: unexpected error: %v", err)
	}
	if got == "" {
		t.Error("expected non-empty link")
	}
}
