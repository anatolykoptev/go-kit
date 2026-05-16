package tgapi5_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/metrics"
	"github.com/anatolykoptev/go-kit/telegram/tgapi5"
)

// getMeResponse is the standard /getMe response used by tgbotapi during NewBotAPIWithAPIEndpoint.
const getMeResponse = `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"TestBot","username":"test_bot"}}`

// mockTGServer returns an httptest.Server that simulates the Telegram Bot API.
// handlers maps path suffix (e.g. "/sendMessage") to a response JSON string.
// If a path is not in handlers it returns {"ok":true,"result":{}}.
func mockTGServer(t *testing.T, handlers map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// tgbotapi calls getMe on init — always handle it.
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		for suffix, body := range handlers {
			if strings.HasSuffix(r.URL.Path, suffix) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
				return
			}
		}
		// Default: ok with empty result object.
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
	}))
	return srv
}

// newTestBot creates a tgbotapi.BotAPI pointed at the given test server URL.
func newTestBot(t *testing.T, serverURL string) *tgbotapi.BotAPI {
	t.Helper()
	bot, err := tgbotapi.NewBotAPIWithAPIEndpoint("test-token", serverURL+"/bot%s/%s")
	if err != nil {
		t.Fatalf("newTestBot: %v", err)
	}
	return bot
}

// metricKey returns the labeled metric key in the format used by metrics.Label.
// e.g. metricKey("foo.total", "result", "ok") => "foo.total{result=ok}"
func metricKey(name, labelKey, labelVal string) string {
	return fmt.Sprintf("%s{%s=%s}", name, labelKey, labelVal)
}

// ---- Sender ----------------------------------------------------------------

func TestSender_Send_success(t *testing.T) {
	sentMsg := `{"ok":true,"result":{"message_id":1,"chat":{"id":123,"type":"private"},"date":0,"text":"hello"}}`
	srv := mockTGServer(t, map[string]string{
		"/sendMessage": sentMsg,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	s := tgapi5.NewSender(bot, m)

	if err := s.Send(123, "hello"); err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}
	// Metric result=ok must be bumped.
	want := metricKey(tgapi5.MetricSendTotal, "result", "ok")
	if got := m.Value(want); got != 1 {
		t.Errorf("metric %s: want 1 got %d", want, got)
	}
}

func TestSender_Send_error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/sendMessage": `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	s := tgapi5.NewSender(bot, m)

	if err := s.Send(999, "fail"); err == nil {
		t.Fatal("Send: expected error, got nil")
	}
	// Metric result=error must be bumped.
	want := metricKey(tgapi5.MetricSendTotal, "result", "error")
	if got := m.Value(want); got != 1 {
		t.Errorf("metric %s: want 1 got %d", want, got)
	}
}

func TestSender_SendChattable_success(t *testing.T) {
	sentMsg := `{"ok":true,"result":{"message_id":2,"chat":{"id":42,"type":"private"},"date":0,"text":"raw"}}`
	srv := mockTGServer(t, map[string]string{
		"/sendMessage": sentMsg,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	s := tgapi5.NewSender(bot, m)

	cfg := tgbotapi.NewMessage(42, "raw")
	if _, err := s.SendChattable(cfg); err != nil {
		t.Fatalf("SendChattable: unexpected error: %v", err)
	}
}

// ---- CallbackAnswerer -------------------------------------------------------

func TestCallbackAnswerer_AnswerCallback_success(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/answerCallbackQuery": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	a := tgapi5.NewCallbackAnswerer(bot)

	if err := a.AnswerCallback("cq-id-1"); err != nil {
		t.Fatalf("AnswerCallback: unexpected error: %v", err)
	}
}

func TestCallbackAnswerer_AnswerCallback_error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/answerCallbackQuery": `{"ok":false,"error_code":400,"description":"QUERY_ID_INVALID"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	a := tgapi5.NewCallbackAnswerer(bot)

	if err := a.AnswerCallback("bad-id"); err == nil {
		t.Fatal("AnswerCallback: expected error, got nil")
	}
}

// ---- MessageDeleter --------------------------------------------------------

func TestMessageDeleter_DeleteMessage_success(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/deleteMessage": `{"ok":true,"result":true}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	d := tgapi5.NewMessageDeleter(bot, m)

	if err := d.DeleteMessage(123, 456); err != nil {
		t.Fatalf("DeleteMessage: unexpected error: %v", err)
	}
	want := metricKey(tgapi5.MetricDeleteTotal, "result", "ok")
	if got := m.Value(want); got != 1 {
		t.Errorf("metric %s: want 1 got %d", want, got)
	}
}

func TestMessageDeleter_DeleteMessage_tooOld(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/deleteMessage": `{"ok":false,"error_code":400,"description":"Bad Request: message can't be deleted for everyone"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	d := tgapi5.NewMessageDeleter(bot, m)

	if err := d.DeleteMessage(123, 456); err == nil {
		t.Fatal("DeleteMessage: expected error for too-old message")
	}
	want := metricKey(tgapi5.MetricDeleteTotal, "result", "too_old")
	if got := m.Value(want); got != 1 {
		t.Errorf("metric %s: want 1 got %d", want, got)
	}
}

func TestMessageDeleter_DeleteMessage_otherError(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/deleteMessage": `{"ok":false,"error_code":400,"description":"Bad Request: something else"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	m := metrics.NewRegistry()
	d := tgapi5.NewMessageDeleter(bot, m)

	if err := d.DeleteMessage(123, 456); err == nil {
		t.Fatal("DeleteMessage: expected error")
	}
	want := metricKey(tgapi5.MetricDeleteTotal, "result", "other_error")
	if got := m.Value(want); got != 1 {
		t.Errorf("metric %s: want 1 got %d", want, got)
	}
}

// ---- InlineAnswerer --------------------------------------------------------

func TestInlineAnswerer_AnswerInlineQuery_success(t *testing.T) {
	var capturedQueryID, capturedCacheTime string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, getMeResponse)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/answerInlineQuery") {
			// tgbotapi sends application/x-www-form-urlencoded, not JSON.
			if err := r.ParseForm(); err == nil {
				capturedQueryID = r.FormValue("inline_query_id")
				capturedCacheTime = r.FormValue("cache_time")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}))
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	a := tgapi5.NewInlineAnswerer(bot)

	results := []tgapi5.InlineResult{
		{ID: "1", Title: "Edge A", MessageText: "edge-a.example.com"},
	}
	if err := a.AnswerInlineQuery("q-id-1", results, 86400, false); err != nil {
		t.Fatalf("AnswerInlineQuery: unexpected error: %v", err)
	}

	// Verify the captured form fields.
	if capturedQueryID != "q-id-1" {
		t.Errorf("inline_query_id: want %q got %q", "q-id-1", capturedQueryID)
	}
	if capturedCacheTime != "86400" {
		t.Errorf("cache_time: want %q got %q", "86400", capturedCacheTime)
	}
}

func TestInlineAnswerer_AnswerInlineQuery_error(t *testing.T) {
	srv := mockTGServer(t, map[string]string{
		"/answerInlineQuery": `{"ok":false,"error_code":400,"description":"QUERY_TOO_OLD"}`,
	})
	defer srv.Close()

	bot := newTestBot(t, srv.URL)
	a := tgapi5.NewInlineAnswerer(bot)

	if err := a.AnswerInlineQuery("old-id", nil, 0, false); err == nil {
		t.Fatal("AnswerInlineQuery: expected error, got nil")
	}
}

// ---- Interface satisfaction compile-time checks ----------------------------
// These var declarations confirm that each concrete type satisfies the
// corresponding interface at compile time. A mismatch causes a build error.

var (
	_ interface{ Send(chatID int64, text string) error }                                             = (*tgapi5.BotSender)(nil)
	_ interface{ AnswerCallback(callbackID string) error }                                           = (*tgapi5.BotCallbackAnswerer)(nil)
	_ interface{ DeleteMessage(chatID int64, messageID int) error }                                  = (*tgapi5.BotMessageDeleter)(nil)
	_ tgapi5.InlineAnswerer                                                                          = (*tgapi5.BotInlineAnswerer)(nil)
)
