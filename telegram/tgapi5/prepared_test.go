package tgapi5_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/miniapp"
	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// ---- BotPreparedSender -------------------------------------------------------

func TestBotPreparedSender_SavePreparedInlineMessage_success(t *testing.T) {
	t.Parallel()

	var (
		capturedUserID     string
		capturedAllowUser  string
		capturedAllowBot   string
		capturedAllowGroup string
		capturedAllowChan  string
		capturedResult     string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/savePreparedInlineMessage") {
			// tgbotapi sends application/x-www-form-urlencoded, not JSON.
			if err := r.ParseForm(); err == nil {
				capturedUserID = r.FormValue("user_id")
				capturedAllowUser = r.FormValue("allow_user_chats")
				capturedAllowBot = r.FormValue("allow_bot_chats")
				capturedAllowGroup = r.FormValue("allow_group_chats")
				capturedAllowChan = r.FormValue("allow_channel_chats")
				capturedResult = r.FormValue("result")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":"prep_abc","expiration_date":1234567890}}`)
	}))
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	s := tgapi5.NewPreparedSender(bot)

	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Test"}
	opts := miniapp.PreparedOptions{AllowUserChats: true, AllowGroupChats: true}

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

	// Body-level assertions: the SDK-compatible param names must reach Telegram.
	// A regression to camelCase or missing key would break the actual Bot API call.
	if capturedUserID != "42" {
		t.Errorf("user_id form field = %q; want %q", capturedUserID, "42")
	}
	// tgbotapi.Params.AddBool omits the key when the value is false (SDK
	// convention — Telegram treats absent fields as false). Verify true flags
	// are present-as-"true" AND false flags are absent.
	if capturedAllowUser != "true" {
		t.Errorf("allow_user_chats = %q; want %q (true flag should be present)", capturedAllowUser, "true")
	}
	if capturedAllowBot != "" {
		t.Errorf("allow_bot_chats = %q; want %q (false flag should be absent)", capturedAllowBot, "")
	}
	if capturedAllowGroup != "true" {
		t.Errorf("allow_group_chats = %q; want %q (true flag should be present)", capturedAllowGroup, "true")
	}
	if capturedAllowChan != "" {
		t.Errorf("allow_channel_chats = %q; want %q (false flag should be absent)", capturedAllowChan, "")
	}
	if capturedResult == "" {
		t.Error("result form field empty; want JSON-marshalled InlineQueryResult")
	}
	if !strings.Contains(capturedResult, `"id":"1"`) || !strings.Contains(capturedResult, `"title":"Test"`) {
		t.Errorf("result JSON missing expected fields: %q", capturedResult)
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
