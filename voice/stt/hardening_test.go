package stt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const secretBody = "secret transcript fragment 4471"

func audioFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(p, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// jsonOfSize returns a valid {"text":"aaa…"} object of exactly n bytes.
func jsonOfSize(n int) []byte {
	const wrap = len(`{"text":""}`)
	return []byte(`{"text":"` + strings.Repeat("a", n-wrap) + `"}`)
}

func serve(t *testing.T, h http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

// call runs one client method by name and returns its error.
func call(c *Client, kind, path string) error {
	ctx := context.Background()
	var err error
	switch kind {
	case "transcribe":
		_, err = c.Transcribe(ctx, path)
	case "verbose":
		_, err = c.TranscribeVerbose(ctx, path)
	case "raw":
		_, err = c.TranscribeRaw(ctx, path)
	case "models":
		_, err = c.Models(ctx)
	}
	return err
}

func TestResponseReadIsBounded(t *testing.T) {
	cases := []struct {
		name, kind, format string
		limit              int
	}{
		{"json", "transcribe", "json", maxTextResponse},
		{"verbose_json", "verbose", "verbose_json", maxVerboseResponse},
		{"raw srt", "raw", "srt", maxVerboseResponse},
		{"raw text", "raw", "text", maxTextResponse},
		{"models", "models", "json", maxModelsResponse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var size atomic.Int64
			url := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(jsonOfSize(int(size.Load())))
			})
			client := New(url, WithFormat(c.format))

			size.Store(int64(c.limit)) // exactly at the cap: accepted (or at least not "too large")
			if err := call(client, c.kind, audioFile(t)); errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("response of exactly the cap was rejected: %v", err)
			}
			size.Store(int64(c.limit) + 1)
			if err := call(client, c.kind, audioFile(t)); !errors.Is(err, ErrResponseTooLarge) {
				t.Errorf("response over the cap: err=%v, want ErrResponseTooLarge", err)
			}
		})
	}
}

func TestErrorBodyReadIsBounded(t *testing.T) {
	var written atomic.Int64
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		chunk := make([]byte, 1<<20)
		for range 128 {
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil || r.Context().Err() != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	})
	err := call(New(url), "transcribe", audioFile(t))
	var se *Error
	if !errors.As(err, &se) || se.StatusCode != http.StatusInternalServerError {
		t.Fatalf("err = %v, want *Error 500", err)
	}
	if got := written.Load(); got > 64<<20 {
		t.Errorf("client let the server write %d bytes of an error body", got)
	}
}

func TestErrorNeverCarriesServerBody(t *testing.T) {
	bodies := map[string]struct{ body, wantReason string }{
		"code token":        {`{"error":{"message":"` + secretBody + `","code":"invalid_api_key"}}`, "invalid_api_key"},
		"type token":        {`{"error":{"message":"` + secretBody + `","type":"server_error"}}`, "server_error"},
		"numeric code":      {`{"error":{"message":"` + secretBody + `","code":429}}`, ""},
		"free-text code":    {`{"error":{"code":"` + secretBody + `"}}`, ""},
		"free-text type":    {`{"error":{"type":"` + secretBody + `"}}`, ""},
		"plain text body":   {secretBody, ""},
		"unparseable json":  {`{"error":` + secretBody, ""},
		"empty body":        {``, ""},
		"top-level message": {`{"message":"` + secretBody + `"}`, ""},
	}
	for name, b := range bodies {
		for _, kind := range []string{"transcribe", "verbose", "raw", "models"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				url := serve(t, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(b.body))
				})
				err := call(New(url), kind, audioFile(t))
				var se *Error
				if !errors.As(err, &se) {
					t.Fatalf("err = %v, want *Error", err)
				}
				if strings.Contains(err.Error(), "secret") || strings.Contains(se.Message, "secret") || strings.Contains(se.Reason, "secret") {
					t.Errorf("server body leaked: %v", err)
				}
				if se.StatusCode != http.StatusInternalServerError || se.Message != "Internal Server Error" || se.Reason != b.wantReason {
					t.Errorf("got %+v, want status 500, status text, reason %q", se, b.wantReason)
				}
			})
		}
	}
}

func TestTranscribeTextFormat(t *testing.T) {
	var gotFormat atomic.Value
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		gotFormat.Store(r.FormValue("response_format"))
		_, _ = w.Write([]byte("  hello world\n"))
	})
	resp, err := New(url, WithFormat("text"), WithLanguage("en")).Transcribe(context.Background(), audioFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "hello world" || resp.Language != "en" || gotFormat.Load() != "text" {
		t.Errorf("resp=%+v format=%v", resp, gotFormat.Load())
	}
}

func TestTranscribeRejectsSubtitleFormats(t *testing.T) {
	for _, f := range []string{"srt", "vtt"} {
		t.Run(f, func(t *testing.T) {
			var reqs atomic.Int32
			url := serve(t, func(http.ResponseWriter, *http.Request) { reqs.Add(1) })
			_, err := New(url, WithFormat(f)).Transcribe(context.Background(), audioFile(t))
			if !errors.Is(err, ErrUnsupportedFormat) || reqs.Load() != 0 {
				t.Errorf("err=%v requests=%d, want ErrUnsupportedFormat and no request", err, reqs.Load())
			}
		})
	}
}

func TestDownloadIsBounded(t *testing.T) {
	var size atomic.Int64
	url := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(size.Load()))))
	})
	tmp := t.TempDir()
	c := New(url, WithTempDir(tmp), WithMaxDownload(10))

	size.Store(11)
	if _, err := c.downloadToTemp(context.Background(), url+"/f.ogg"); !errors.Is(err, ErrResponseTooLarge) {
		t.Errorf("over the cap: err=%v", err)
	}
	if es, _ := os.ReadDir(tmp); len(es) != 0 {
		t.Errorf("oversized download left %d files", len(es))
	}
	size.Store(10)
	p, err := c.downloadToTemp(context.Background(), url+"/f.ogg")
	if err != nil {
		t.Fatalf("at the cap: %v", err)
	}
	os.Remove(p)
}

// streamForever serves 1 MiB chunks (up to 128 MiB) with status 200 and counts
// how much the client let it write: a bounded reader stops pulling near the
// cap, an unbounded one drains everything.
func streamForever(t *testing.T, written *atomic.Int64) string {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 1<<20)
		for range 128 {
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil || r.Context().Err() != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	})
}

func TestResponseReadStopsPullingAtCap(t *testing.T) {
	var written atomic.Int64
	err := call(New(streamForever(t, &written)), "transcribe", audioFile(t))
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
	if got := written.Load(); got > 64<<20 {
		t.Errorf("client pulled %d bytes past a %d-byte cap", got, maxTextResponse)
	}
}

func TestDownloadStopsPullingAtCap(t *testing.T) {
	var written atomic.Int64
	url := streamForever(t, &written)
	if _, err := New(url, WithTempDir(t.TempDir()), WithMaxDownload(10)).downloadToTemp(context.Background(), url+"/f.ogg"); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
	if got := written.Load(); got > 64<<20 {
		t.Errorf("client pulled %d bytes past a 10-byte cap", got)
	}
}
