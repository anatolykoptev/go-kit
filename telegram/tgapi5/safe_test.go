package tgapi5

import (
	"bytes"
	"errors"
	"io"
	stdlog "log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/tgsafe"
)

const Placeholder = tgsafe.Placeholder

const (
	fakeToken = "123456789:AAFakeTokenFakeTokenFake_x-y"
	getMeOK   = `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"b","username":"b"}}`
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// restoreSDKLogger puts the SDK's package logger back to its default after a test.
func restoreSDKLogger(t *testing.T) {
	t.Cleanup(func() {
		resetSafeLogger()
		_ = tgbotapi.SetLogger(stdlog.New(os.Stderr, "", stdlog.LstdFlags))
	})
}

// fakeTelegram answers getMe, and getUpdates according to updates.
func fakeTelegram(t *testing.T, updates http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(getMeOK))
			return
		}
		updates(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func endpoint(srv *httptest.Server) string { return srv.URL + "/bot%s/%s" }

func noToken(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, "AAFake") || strings.Contains(s, fakeToken) {
		t.Errorf("%s contains the bot token: %s", what, s)
	}
}

func TestSafeBotAPIConstructionErrorIsScrubbed(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	ep := endpoint(dead)
	dead.Close() // connection refused on the getMe done by the constructor

	_, leaky := tgbotapi.NewBotAPIWithClient(fakeToken, ep, &http.Client{})
	if leaky == nil || !strings.Contains(leaky.Error(), "AAFake") {
		t.Fatalf("control: the plain client should leak the token, got %v", leaky)
	}

	_, err := newSafeBotAPI(fakeToken, ep)
	if err == nil {
		t.Fatal("want an error")
	}
	noToken(t, "constructor error", err.Error())
	var ue *url.Error
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), "bot<redacted>") {
		t.Errorf("err = %v, want a *url.Error with the redacted segment", err)
	}
}

func TestSafeBotAPITimeoutKeepsTypeAndScrubs(t *testing.T) {
	srv := fakeTelegram(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // lets the server notice the client disconnect
		select {                           // hang until the client gives up
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	base := &http.Client{Timeout: 150 * time.Millisecond}

	plain, err := tgbotapi.NewBotAPIWithClient(fakeToken, endpoint(srv), base)
	if err != nil {
		t.Fatal(err)
	}
	if _, leaky := plain.GetUpdates(tgbotapi.UpdateConfig{}); leaky == nil || !strings.Contains(leaky.Error(), "AAFake") {
		t.Fatalf("control: plain client should leak, got %v", leaky)
	}

	bot, err := newSafeBotAPI(fakeToken, endpoint(srv), WithBaseClient(base))
	if err != nil {
		t.Fatal(err)
	}
	_, err = bot.GetUpdates(tgbotapi.UpdateConfig{})
	var ue *url.Error
	if err == nil || !errors.As(err, &ue) || !ue.Timeout() {
		t.Fatalf("err = %v, want a *url.Error with Timeout() true", err)
	}
	noToken(t, "timeout error", err.Error())
}

func TestSafeBotAPIDefaultTimeout(t *testing.T) {
	srv := fakeTelegram(t, nil)
	bot, err := newSafeBotAPI(fakeToken, endpoint(srv))
	if err != nil {
		t.Fatal(err)
	}
	hc, ok := bot.Client.(*tgsafe.HTTPClient)
	if !ok || hc.Client().Timeout != DefaultSafeTimeout || DefaultSafeTimeout <= 60*time.Second {
		t.Errorf("client = %T timeout=%v, want a scrubbing client with a timeout above the 60s long-poll", bot.Client, hc.Client().Timeout)
	}
}

// pollUntilLogged runs the real GetUpdatesChan against a server that has gone
// away and waits for the SDK to print its failed-poll lines.
func pollUntilLogged(t *testing.T, bot *tgbotapi.BotAPI, out *syncBuf) {
	t.Helper()
	ch := bot.GetUpdatesChan(tgbotapi.UpdateConfig{})
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "Failed to get updates") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	bot.StopReceivingUpdates()
	for range ch { // the poll goroutine closes ch on exit; only then is the global logger safe to swap back
	}
	if !strings.Contains(out.String(), "Failed to get updates") {
		t.Fatalf("the SDK never logged a failed poll: %q", out.String())
	}
}

func TestGetUpdatesChanFailedPollIsScrubbedByTheLogger(t *testing.T) {
	restoreSDKLogger(t)
	resetSafeLogger()

	t.Run("control: default SDK logging leaks", func(t *testing.T) {
		srv := fakeTelegram(t, nil)
		bot, err := tgbotapi.NewBotAPIWithClient(fakeToken, endpoint(srv), &http.Client{})
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
		var out syncBuf
		_ = tgbotapi.SetLogger(stdlog.New(&out, "", 0))
		pollUntilLogged(t, bot, &out)
		if !strings.Contains(out.String(), "AAFake") {
			t.Fatalf("control did not leak, the test proves nothing: %s", out.String())
		}
	})

	t.Run("safe client plus safe logger", func(t *testing.T) {
		srv := fakeTelegram(t, nil)
		bot, err := newSafeBotAPI(fakeToken, endpoint(srv))
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
		var out syncBuf
		resetSafeLogger()
		if err := SetSafeLogger(slog.New(slog.NewTextHandler(&out, nil)), fakeToken); err != nil {
			t.Fatal(err)
		}
		pollUntilLogged(t, bot, &out)
		noToken(t, "logger output", out.String())
		if !strings.Contains(out.String(), "bot<redacted>") {
			t.Errorf("redacted URL missing from %s", out.String())
		}
	})

	t.Run("safe logger alone covers a plain client", func(t *testing.T) {
		srv := fakeTelegram(t, nil)
		bot, err := tgbotapi.NewBotAPIWithClient(fakeToken, endpoint(srv), &http.Client{})
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
		var out syncBuf
		resetSafeLogger()
		_ = SetSafeLogger(slog.New(slog.NewTextHandler(&out, nil)))
		pollUntilLogged(t, bot, &out)
		noToken(t, "logger output", out.String())
	})
}

// Two bots with different tokens: a second SetSafeLogger call must add its
// secret to the one installed logger, not replace it. The tokens are not
// bot<id>:<secret> shaped, so only the literal-secret path can scrub them.
func TestSetSafeLoggerAccumulatesSecretsAndKeepsTheFirstLogger(t *testing.T) {
	restoreSDKLogger(t)
	resetSafeLogger()
	const tokA, tokB = "literalTokenAAA", "literalTokenBBB"

	mkBot := func(tok string) *tgbotapi.BotAPI {
		srv := fakeTelegram(t, nil)
		bot, err := tgbotapi.NewBotAPIWithClient(tok, endpoint(srv), &http.Client{})
		if err != nil {
			t.Fatal(err)
		}
		srv.Close()
		return bot
	}
	botA, botB := mkBot(tokA), mkBot(tokB)

	var first, second syncBuf
	if err := SetSafeLogger(slog.New(slog.NewTextHandler(&first, nil)), tokA); err != nil {
		t.Fatal(err)
	}
	if err := SetSafeLogger(slog.New(slog.NewTextHandler(&second, nil)), tokB); err != nil {
		t.Fatal(err)
	}
	pollUntilLogged(t, botA, &first)
	// botB's failed poll is logged after botA's goroutine is gone; wait on a fresh marker.
	before := len(first.String())
	ch := botB.GetUpdatesChan(tgbotapi.UpdateConfig{})
	deadline := time.Now().Add(5 * time.Second)
	for len(first.String()) == before && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	botB.StopReceivingUpdates()
	for range ch {
	}

	out := first.String()
	if strings.Contains(out, tokA) || strings.Contains(out, tokB) {
		t.Errorf("a token reached the log: %s", out)
	}
	if !strings.Contains(out, Placeholder) {
		t.Errorf("nothing was redacted (the poll never logged?): %s", out)
	}
	if second.String() != "" {
		t.Errorf("second call swapped the logger destination: %s", second.String())
	}
}

// Adding secrets while an SDK poll goroutine is logging must not race (run with -race).
func TestSetSafeLoggerAfterPollingStartedDoesNotRace(t *testing.T) {
	restoreSDKLogger(t)
	resetSafeLogger()
	srv := fakeTelegram(t, nil)
	bot, err := tgbotapi.NewBotAPIWithClient("literalTokenCCC", endpoint(srv), &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	var out syncBuf
	_ = SetSafeLogger(slog.New(slog.NewTextHandler(&out, nil)), "literalTokenCCC")
	ch := bot.GetUpdatesChan(tgbotapi.UpdateConfig{})
	for range 100 {
		_ = SetSafeLogger(nil, "another-secret")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(out.String(), "Failed to get updates") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	bot.StopReceivingUpdates()
	for range ch {
	}
}
