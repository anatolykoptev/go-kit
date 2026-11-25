package tgapi5

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/files"
)

type countingClient struct {
	n atomic.Int32
	c *http.Client
}

func (c *countingClient) Do(r *http.Request) (*http.Response, error) {
	c.n.Add(1)
	return c.c.Do(r)
}

func TestNewFileDownloaderCopiesBotSettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"b","username":"b"}}`)
		case r.URL.Path == "/bot"+fakeToken+"/getFile":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"file_id":"f","file_unique_id":"u","file_size":2,"file_path":"voice/a.oga"}}`)
		case r.URL.Path == "/file/bot"+fakeToken+"/voice/a.oga":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cc := &countingClient{c: srv.Client()}
	bot, err := tgbotapi.NewBotAPIWithOptions(fakeToken,
		tgbotapi.WithAPIEndpoint(srv.URL+"/bot%s/%s"), tgbotapi.WithHTTPClient(cc))
	if err != nil {
		t.Fatal(err)
	}
	bot.SetFileEndpoint(srv.URL + "/file/bot%s/%s") // a self-hosted Bot API server
	before := cc.n.Load()

	d, err := NewFileDownloader(bot)
	if err != nil {
		t.Fatal(err)
	}
	rc, info, err := d.Download(context.Background(), "f")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if b, _ := io.ReadAll(rc); string(b) != "ok" || info.FileUniqueID != "u" {
		t.Errorf("body %q info %+v", b, info)
	}
	if cc.n.Load() == before {
		t.Error("the bot's HTTP client was not used")
	}
}

func TestNewFileDownloaderErrors(t *testing.T) {
	if _, err := NewFileDownloader(nil); !errors.Is(err, ErrNilBot) {
		t.Errorf("nil bot: %v", err)
	}
	if _, err := NewFileDownloader(&tgbotapi.BotAPI{Token: fakeToken}); !errors.Is(err, ErrFileEndpoint) {
		t.Errorf("zero bot: %v", err)
	}
	if _, err := NewFileDownloader(&tgbotapi.BotAPI{}); !errors.Is(err, ErrFileEndpoint) {
		t.Errorf("empty token: %v", err)
	}
	bot := &tgbotapi.BotAPI{Token: fakeToken}
	bot.SetFileEndpoint("https://example.org/dl/%s/%s")
	if _, err := NewFileDownloader(bot); err == nil || strings.Contains(err.Error(), fakeToken) {
		t.Errorf("odd endpoint: %v", err)
	}
	// overriding options still apply
	bot.SetFileEndpoint(tgbotapi.FileEndpoint)
	if _, err := NewFileDownloader(bot, files.WithMaxSize(1)); err != nil {
		t.Errorf("default endpoint: %v", err)
	}
}
