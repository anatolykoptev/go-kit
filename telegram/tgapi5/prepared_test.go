package tgapi5_test

import (
	"context"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/miniapp"
	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// ---- BotPreparedSender -------------------------------------------------------

func TestBotPreparedSender_SavePreparedInlineMessage_success(t *testing.T) {
	t.Parallel()
	srv := mockTGServer(t, map[string]string{
		"/savePreparedInlineMessage": `{"ok":true,"result":{"id":"prep_abc","expiration_date":1234567890}}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	s := tgapi5.NewPreparedSender(bot)

	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Test"}
	opts := miniapp.PreparedOptions{AllowUserChats: true}

	got, err := s.SavePreparedInlineMessage(context.Background(), 42, result, opts)
	if err != nil {
		t.Fatalf("SavePreparedInlineMessage: unexpected error: %v", err)
	}
	if got.ID != "prep_abc" {
		t.Errorf("ID = %q; want %q", got.ID, "prep_abc")
	}
	if got.ExpirationDate != 1234567890 {
		t.Errorf("ExpirationDate = %d; want 1234567890", got.ExpirationDate)
	}
}

func TestBotPreparedSender_SavePreparedInlineMessage_error(t *testing.T) {
	t.Parallel()
	srv := mockTGServer(t, map[string]string{
		"/savePreparedInlineMessage": `{"ok":false,"error_code":400,"description":"Bad Request: user not found"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	s := tgapi5.NewPreparedSender(bot)

	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Test"}
	opts := miniapp.PreparedOptions{AllowUserChats: true}

	_, err := s.SavePreparedInlineMessage(context.Background(), 42, result, opts)
	if err == nil {
		t.Fatal("SavePreparedInlineMessage: expected error, got nil")
	}
}
