package files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeToken is fixed so the leak assertions are exact; its shape matches a real
// bot token, so the shape-based scrub also applies.
const fakeToken = "123456789:AAFakeTokenFakeTokenFakeToken_abc"

const (
	fileID     = "AgAD-file-id"
	uniqueID   = "AQAD-unique"
	voicePath  = "voice/file_7.oga"
	voiceBytes = "OggS-fake-voice-bytes"
)

// fakeTG serves getFile and the file download like the Bot API. Handlers can be
// swapped per test through the two fields.
type fakeTG struct {
	srv      *httptest.Server
	getFile  func(w http.ResponseWriter, r *http.Request)
	download func(w http.ResponseWriter, r *http.Request)
}

func newFakeTG(t *testing.T) *fakeTG {
	t.Helper()
	f := &fakeTG{}
	f.getFile = func(w http.ResponseWriter, _ *http.Request) { writeGetFile(w, int64(len(voiceBytes)), voicePath) }
	f.download = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/ogg")
		_, _ = io.WriteString(w, voiceBytes)
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bot"+fakeToken+"/getFile":
			var in struct {
				FileID string `json:"file_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.FileID != fileID {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"Bad Request: invalid file_id"}`)
				return
			}
			f.getFile(w, r)
		case strings.HasPrefix(r.URL.Path, "/file/bot"+fakeToken+"/"):
			f.download(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func writeGetFile(w http.ResponseWriter, size int64, filePath string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
		"result": map[string]any{
			"file_id": fileID, "file_unique_id": uniqueID, "file_size": size, "file_path": filePath,
		},
	})
}

func (f *fakeTG) dl(opts ...Option) *Downloader {
	return New(fakeToken, append([]Option{WithBaseURL(f.srv.URL)}, opts...)...)
}

func noToken(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	for _, s := range []string{
		fakeToken, "AAFakeToken", "123456789:", "/file/bot" + fakeToken,
		url.QueryEscape(fakeToken),
	} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error leaks %q: %q", s, err)
		}
	}
	// The chain must not hand the original URL back either.
	var ue *url.Error
	for e := error(err); e != nil; e = errors.Unwrap(e) {
		if errors.As(e, &ue) {
			if strings.Contains(ue.URL, fakeToken) || strings.Contains(ue.Error(), fakeToken) {
				t.Errorf("chain holds a *url.Error with the token: %q", ue.URL)
			}
		}
	}
}

func TestDownloadOK(t *testing.T) {
	f := newFakeTG(t)
	rc, info, err := f.dl().Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != voiceBytes {
		t.Errorf("body = %q, want %q", got, voiceBytes)
	}
	want := Info{FileID: fileID, FileUniqueID: uniqueID, Size: int64(len(voiceBytes)), MimeType: "audio/ogg"}
	if info != want {
		t.Errorf("info = %+v, want %+v", info, want)
	}
	if s := fmt.Sprintf("%+v", info); strings.Contains(s, voicePath) || strings.Contains(s, fakeToken) {
		t.Errorf("Info exposes the path or token: %s", s)
	}
}

func TestDownloadMimeFallsBackToExtension(t *testing.T) {
	f := newFakeTG(t)
	f.getFile = func(w http.ResponseWriter, _ *http.Request) { writeGetFile(w, 3, "photos/file_1.jpg") }
	f.download = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, "jpg")
	}
	rc, info, err := f.dl().Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	if info.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q, want image/jpeg", info.MimeType)
	}
}

func TestDownloadToTemp(t *testing.T) {
	f := newFakeTG(t)
	dir := t.TempDir()
	p, cleanup, err := f.dl().DownloadToTemp(context.Background(), fileID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != dir || filepath.Ext(p) != ".oga" {
		t.Errorf("path = %q, want a .oga file in %s", p, dir)
	}
	if b, _ := os.ReadFile(p); string(b) != voiceBytes {
		t.Errorf("file = %q", b)
	}
	_ = cleanup()
	_ = cleanup() // idempotent
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("cleanup left the file: %v", err)
	}
}

func TestDownloadToTempFailureLeavesNothing(t *testing.T) {
	f := newFakeTG(t)
	f.download = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 100)) }
	dir := t.TempDir()
	_, cleanup, err := f.dl(WithMaxSize(10)).DownloadToTemp(context.Background(), fileID, dir)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	_ = cleanup()
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("temp dir not empty after a failed download: %d entries", len(ents))
	}
}

func TestOversizeRefusedBeforeRead(t *testing.T) {
	f := newFakeTG(t)
	f.getFile = func(w http.ResponseWriter, _ *http.Request) { writeGetFile(w, 1000, voicePath) }
	downloads := 0
	f.download = func(http.ResponseWriter, *http.Request) { downloads++ }
	_, _, err := f.dl(WithMaxSize(999)).Download(context.Background(), fileID)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if downloads != 0 {
		t.Errorf("file endpoint hit %d times; the declared size should refuse first", downloads)
	}
	noToken(t, err)
}

func TestOversizeContentLengthRefused(t *testing.T) {
	f := newFakeTG(t)
	f.getFile = func(w http.ResponseWriter, _ *http.Request) { writeGetFile(w, 0, voicePath) } // size unknown
	f.download = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, strings.Repeat("x", 100)) }
	_, _, err := f.dl(WithMaxSize(10)).Download(context.Background(), fileID)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestOversizeCutOffDuringRead(t *testing.T) {
	f := newFakeTG(t)
	// getFile claims 5 bytes and so does the first chunk's declared length is
	// unknown (chunked): the server just keeps sending.
	f.getFile = func(w http.ResponseWriter, _ *http.Request) { writeGetFile(w, 5, voicePath) }
	f.download = func(w http.ResponseWriter, _ *http.Request) {
		fl := w.(http.Flusher)
		for range 50 {
			_, _ = io.WriteString(w, "0123456789")
			fl.Flush()
		}
	}
	const limit = 25
	rc, _, err := f.dl(WithMaxSize(limit)).Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("read err = %v, want ErrTooLarge", err)
	}
	if len(got) > limit {
		t.Errorf("delivered %d bytes, limit %d", len(got), limit)
	}
	noToken(t, err)
}

func TestExactLimitIsAllowed(t *testing.T) {
	f := newFakeTG(t)
	rc, _, err := f.dl(WithMaxSize(int64(len(voiceBytes)))).Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if b, err := io.ReadAll(rc); err != nil || string(b) != voiceBytes {
		t.Errorf("read = %q, %v; a file exactly at the limit must pass", b, err)
	}
}

// Every failure path: the error must not carry the token. Each fake plants the
// token (or the file URL) into the place its error text is built from, the way a
// reverse proxy that echoes the request path would, so a missing scrub shows.
func TestFailureErrorsNeverContainToken(t *testing.T) {
	echo := "Not Found: /file/bot" + fakeToken + "/" + voicePath

	t.Run("connection refused", func(t *testing.T) {
		f := newFakeTG(t)
		d := f.dl()
		f.srv.Close()
		_, _, err := d.Download(context.Background(), fileID)
		noToken(t, err)
		var ue *url.Error
		if !errors.As(err, &ue) || !strings.Contains(ue.URL, "bot<redacted>") {
			t.Errorf("want a *url.Error with the redacted URL, got %#v", err)
		}
	})

	t.Run("connection refused on download", func(t *testing.T) {
		f := newFakeTG(t)
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Path, "/file/bot") {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
			}
			return http.DefaultTransport.RoundTrip(r)
		})
		_, _, err := f.dl(WithHTTPClient(&http.Client{Transport: rt})).Download(context.Background(), fileID)
		if !errors.Is(err, syscall.ECONNREFUSED) {
			t.Fatalf("err = %v, want ECONNREFUSED in the chain", err)
		}
		noToken(t, err)
	})

	t.Run("getFile 404 echoing the path", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": 404, "description": echo})
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		var te *Error
		if !errors.As(err, &te) || te.StatusCode != http.StatusNotFound {
			t.Fatalf("err = %v, want *Error 404", err)
		}
		noToken(t, err)
	})

	t.Run("download 404 echoing the URL", func(t *testing.T) {
		f := newFakeTG(t)
		f.download = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, echo)
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		var te *Error
		if !errors.As(err, &te) || te.StatusCode != http.StatusNotFound || te.Op != "download" {
			t.Fatalf("err = %v, want download *Error 404", err)
		}
		noToken(t, err)
	})

	t.Run("timeout", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}
		_, _, err := f.dl(WithTimeout(50*time.Millisecond)).Download(context.Background(), fileID)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want DeadlineExceeded", err)
		}
		noToken(t, err)
		var ue *url.Error
		if !errors.As(err, &ue) || !ue.Timeout() {
			t.Errorf("redaction must keep the *url.Error and its Timeout(): %#v", err)
		}
	})

	t.Run("timeout while reading the body", func(t *testing.T) {
		f := newFakeTG(t)
		f.download = func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}
		rc, _, err := f.dl(WithTimeout(100*time.Millisecond)).Download(context.Background(), fileID)
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		_, err = io.ReadAll(rc)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("read err = %v, want DeadlineExceeded", err)
		}
		noToken(t, err)
	})

	t.Run("bad getFile body: not JSON", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "<html>proxy error for /bot"+fakeToken+"/getFile</html>")
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		if err == nil || !strings.Contains(err.Error(), "unreadable response") {
			t.Fatalf("err = %v, want an unreadable-response error", err)
		}
		noToken(t, err)
	})

	t.Run("bad getFile body: no file_path", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"ok":true,"result":{"file_id":"x","note":"`+echo+`"}}`)
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		if err == nil || !strings.Contains(err.Error(), "no file_path") {
			t.Fatalf("err = %v, want a no-file_path error", err)
		}
		noToken(t, err)
	})

	t.Run("getFile ok=false with HTTP 200", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"ok":false,"error_code":400,"description":"`+echo+`"}`)
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		var te *Error
		if !errors.As(err, &te) || te.StatusCode != http.StatusBadRequest {
			t.Fatalf("err = %v, want *Error 400", err)
		}
		noToken(t, err)
	})

	t.Run("unknown file_id", func(t *testing.T) {
		f := newFakeTG(t)
		_, _, err := f.dl().Download(context.Background(), "nope")
		var te *Error
		if !errors.As(err, &te) || te.StatusCode != http.StatusBadRequest {
			t.Fatalf("err = %v, want *Error 400", err)
		}
		noToken(t, err)
	})

	t.Run("redirect is not followed", func(t *testing.T) {
		f := newFakeTG(t)
		f.download = func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://elsewhere.invalid"+r.URL.Path, http.StatusFound)
		}
		_, _, err := f.dl().Download(context.Background(), fileID)
		var te *Error
		if !errors.As(err, &te) || te.StatusCode != http.StatusFound {
			t.Fatalf("err = %v, want *Error 302", err)
		}
		noToken(t, err)
	})

	t.Run("invalid base URL", func(t *testing.T) {
		_, _, err := New(fakeToken, WithBaseURL("http://bad host")).Download(context.Background(), fileID)
		if !errors.Is(err, ErrBaseURL) {
			t.Fatalf("err = %v, want ErrBaseURL", err)
		}
		noToken(t, err)
	})

	t.Run("download to temp: bad directory", func(t *testing.T) {
		f := newFakeTG(t)
		_, cleanup, err := f.dl().DownloadToTemp(context.Background(), fileID, filepath.Join(t.TempDir(), "missing", fakeToken))
		_ = cleanup()
		noToken(t, err)
	})
}

func TestArgumentValidation(t *testing.T) {
	if _, _, err := New("").Download(context.Background(), fileID); !errors.Is(err, ErrNoToken) {
		t.Errorf("empty token: %v", err)
	}
	if _, _, err := New(fakeToken).Download(context.Background(), ""); !errors.Is(err, ErrEmptyFileID) {
		t.Errorf("empty file_id: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
