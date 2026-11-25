package files_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/anatolykoptev/go-kit/telegram/files"
	"github.com/anatolykoptev/go-kit/voice/stt"
)

const exampleToken = "123456789:AAFakeTokenFakeTokenFakeToken_abc"

// fakeTelegram serves getFile and a download for one voice note.
func fakeTelegram() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			_, _ = io.WriteString(w, `{"ok":true,"result":{"file_id":"v1","file_unique_id":"u1","file_size":9,"file_path":"voice/v.oga"}}`)
		case strings.HasPrefix(r.URL.Path, "/file/bot"):
			_, _ = io.WriteString(w, "OggS-voice")
		default:
			http.NotFound(w, r)
		}
	}))
}

func fakeSTT() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(f)
		_, _ = fmt.Fprintf(w, `{"text":"heard %d bytes"}`, len(b))
	}))
}

// Download a voice note by file_id and stream it straight into the STT client:
// no URL, no temp file, no token in the caller's hands.
func Example_transcribeVoice() {
	tg, sttSrv := fakeTelegram(), fakeSTT()
	defer tg.Close()
	defer sttSrv.Close()

	ctx := context.Background()
	d := files.New(exampleToken, files.WithBaseURL(tg.URL), files.WithMaxSize(10<<20))
	body, info, err := d.Download(ctx, "v1")
	if err != nil {
		fmt.Println("download:", err)
		return
	}
	defer body.Close()

	resp, err := stt.New(sttSrv.URL).TranscribeReader(ctx, body, "voice.oga")
	if err != nil {
		fmt.Println("stt:", err)
		return
	}
	fmt.Println(info.FileUniqueID, resp.Text)
	// Output: u1 heard 10 bytes
}
