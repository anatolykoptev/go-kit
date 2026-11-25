package tgapi5

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/files"
)

func TestNewFileDownloaderUsesBotToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bot" + fakeToken + "/getFile":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"file_id":"f","file_unique_id":"u","file_size":2,"file_path":"voice/a.oga"}}`)
		case "/file/bot" + fakeToken + "/voice/a.oga":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := NewFileDownloader(&tgbotapi.BotAPI{Token: fakeToken}, files.WithBaseURL(srv.URL))
	rc, info, err := d.Download(context.Background(), "f")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if b, _ := io.ReadAll(rc); string(b) != "ok" || info.FileUniqueID != "u" {
		t.Errorf("body %q info %+v", b, info)
	}
}
