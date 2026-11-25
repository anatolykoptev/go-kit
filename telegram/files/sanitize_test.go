package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func leaksToken(s string) bool {
	return strings.Contains(s, fakeToken) || strings.Contains(s, "AAFakeToken") || strings.Contains(s, "123456789")
}

func TestDownloaderNeverPrintsTokenInAnyVerb(t *testing.T) {
	d := New(fakeToken, WithBaseURL("http://127.0.0.1:1"))
	type holder struct {
		D  Downloader
		P  *Downloader
		In any
	}
	h := holder{D: *d, P: d, In: d}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%t", "%p", "%10.3v", "%T", "%e"} {
		for name, arg := range map[string]any{"ptr": d, "value": *d, "state": d.s, "holder": h, "slice": []*Downloader{d}, "map": map[string]*Downloader{"k": d}} {
			if out := fmt.Sprintf(verb, arg); leaksToken(out) {
				t.Errorf("%s of %s leaks: %q", verb, name, out)
			}
		}
	}
	for name, s := range map[string]string{
		"Sprint": fmt.Sprint(d), "Sprintln": fmt.Sprintln(*d), "String": d.String(), "GoString": d.GoString(),
	} {
		if leaksToken(s) {
			t.Errorf("%s leaks: %q", name, s)
		}
	}
	var sb strings.Builder
	slog.New(slog.NewJSONHandler(&sb, nil)).Info("x", "dl", d, "dl_value", *d, slog.Any("grp", h))
	slog.New(slog.NewTextHandler(&sb, nil)).Info("x", "dl", d, "dl_value", *d)
	if leaksToken(sb.String()) {
		t.Errorf("slog leaks: %s", sb.String())
	}
}

func TestScrubBeforeTruncate(t *testing.T) {
	pad := strings.Repeat("x", descLen-20) // the 160-byte cut falls inside the token
	cases := map[string]string{
		"raw":                pad + fakeToken + " tail",
		"path":               pad + "/file/bot" + fakeToken + "/a.oga",
		"percent-encoded":    pad + url.QueryEscape(fakeToken),
		"lower-hex encoded":  pad + strings.ReplaceAll(url.QueryEscape(fakeToken), "%3A", "%3a"),
		"path-escaped":       pad + url.PathEscape(fakeToken),
		"double-encoded":     pad + url.QueryEscape(url.QueryEscape(fakeToken)),
		"every byte encoded": pad + encodeAll(fakeToken),
		"short, encoded":     "echo /bot" + encodeAll(fakeToken) + "/getFile",
	}
	for name, desc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFakeTG(t)
			f.getFile = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = fmt.Fprintf(w, `{"ok":false,"error_code":502,"description":%q}`, desc)
			}
			_, _, err := f.dl().Download(context.Background(), fileID)
			noToken(t, err)
			for _, frag := range []string{"AAFakeTok", "123456789", "FakeTokenFake", "%3AAAF", "%3aAAF"} {
				if strings.Contains(err.Error(), frag) {
					t.Errorf("fragment %q survived: %q", frag, err)
				}
			}
		})
	}

	t.Run("non-JSON body snippet straddling the cap", func(t *testing.T) {
		f := newFakeTG(t)
		f.getFile = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, pad+fakeToken+" more") }
		_, _, err := f.dl().Download(context.Background(), fileID)
		noToken(t, err)
		if strings.Contains(err.Error(), "AAFakeTok") {
			t.Errorf("fragment survived: %q", err)
		}
	})
}

func encodeAll(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		fmt.Fprintf(&sb, "%%%02X", s[i])
	}
	return sb.String()
}

// opaque has a clean message but wraps a raw *url.Error that carries the token.
type opaque struct{ inner error }

func (o *opaque) Error() string { return "request failed" }
func (o *opaque) Unwrap() error { return o.inner }

func rawURLError(inner error) error {
	return &opaque{inner: &url.Error{Op: "Get", URL: "https://api.telegram.org/file/bot" + fakeToken + "/a.oga", Err: inner}}
}

type bodyFunc struct {
	read  func([]byte) (int, error)
	close func() error
}

func (b bodyFunc) Read(p []byte) (int, error) { return b.read(p) }
func (b bodyFunc) Close() error               { return b.close() }

// doerFunc serves getFile itself and hands the download a scripted body.
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func scriptedDoer(body io.ReadCloser) doerFunc {
	return func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/getFile") {
			return &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{},
				Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"file_id":"f","file_unique_id":"u","file_size":3,"file_path":"voice/a.oga"}}`)),
			}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, ContentLength: -1, Body: body}, nil
	}
}

func TestWrappedRawURLErrorsAreCutAtEveryBoundary(t *testing.T) {
	assertCut := func(t *testing.T, err error, is error) {
		t.Helper()
		noToken(t, err)
		var ue *url.Error
		if errors.As(err, &ue) {
			t.Errorf("chain still hands out a *url.Error: %q", ue.URL)
		}
		if is != nil && !errors.Is(err, is) {
			t.Errorf("lost the class %v: %v", is, err)
		}
	}
	base := "http://127.0.0.1:1"

	t.Run("Read", func(t *testing.T) {
		body := bodyFunc{
			read:  func([]byte) (int, error) { return 0, rawURLError(context.DeadlineExceeded) },
			close: func() error { return nil },
		}
		rc, _, err := New(fakeToken, WithBaseURL(base), WithHTTPClient(scriptedDoer(body))).Download(context.Background(), "f")
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		_, err = io.ReadAll(rc)
		assertCut(t, err, context.DeadlineExceeded)
		var to interface{ Timeout() bool }
		if !errors.As(err, &to) || !to.Timeout() {
			t.Errorf("Timeout() lost: %v", err)
		}
	})

	t.Run("Read wrapped EOF is not a clean EOF", func(t *testing.T) {
		body := bodyFunc{
			read:  func([]byte) (int, error) { return 0, rawURLError(io.EOF) },
			close: func() error { return nil },
		}
		rc, _, err := New(fakeToken, WithBaseURL(base), WithHTTPClient(scriptedDoer(body))).Download(context.Background(), "f")
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		_, err = io.ReadAll(rc)
		assertCut(t, err, nil)
	})

	t.Run("Close", func(t *testing.T) {
		body := bodyFunc{
			read:  func([]byte) (int, error) { return 0, io.EOF },
			close: func() error { return rawURLError(context.Canceled) },
		}
		rc, _, err := New(fakeToken, WithBaseURL(base), WithHTTPClient(scriptedDoer(body))).Download(context.Background(), "f")
		if err != nil {
			t.Fatal(err)
		}
		assertCut(t, rc.Close(), context.Canceled)
	})

	t.Run("Do", func(t *testing.T) {
		d := New(fakeToken, WithBaseURL(base), WithHTTPClient(doerFunc(func(*http.Request) (*http.Response, error) {
			return nil, rawURLError(context.Canceled)
		})))
		_, _, err := d.Download(context.Background(), "f")
		assertCut(t, err, context.Canceled)
	})

	t.Run("DownloadToTemp copy error", func(t *testing.T) {
		body := bodyFunc{
			read:  func([]byte) (int, error) { return 0, rawURLError(context.Canceled) },
			close: func() error { return nil },
		}
		dir := t.TempDir()
		_, _, err := New(fakeToken, WithBaseURL(base), WithHTTPClient(scriptedDoer(body))).DownloadToTemp(context.Background(), "f", dir)
		assertCut(t, err, context.Canceled)
		if ents, _ := os.ReadDir(dir); len(ents) != 0 {
			t.Errorf("temp file left behind: %d", len(ents))
		}
	})
}

func TestBaseURLScheme(t *testing.T) {
	for _, bad := range []string{
		"http://api.telegram.org", "http://example.com", "http://10.0.0.5:8081", "ftp://127.0.0.1",
		"https://user:pw@api.telegram.org", "api.telegram.org", "http://", "://x",
	} {
		_, _, err := New(fakeToken, WithBaseURL(bad)).Download(context.Background(), fileID)
		if !errors.Is(err, ErrBaseURL) {
			t.Errorf("%q: err = %v, want ErrBaseURL", bad, err)
		} else if strings.Contains(err.Error(), "example.com") {
			t.Errorf("%q: error echoes the URL: %v", bad, err)
		}
	}
	for _, ok := range []string{"http://127.0.0.1:8081", "http://localhost:8081", "http://[::1]:8081", "https://tg.example.org"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // never touch the network
		_, _, err := New(fakeToken, WithBaseURL(ok)).Download(ctx, fileID)
		if errors.Is(err, ErrBaseURL) {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
}

func TestMaxSizeNearMaxInt64DoesNotOverflow(t *testing.T) {
	f := newFakeTG(t)
	rc, _, err := f.dl(WithMaxSize(math.MaxInt64)).Download(context.Background(), fileID)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if b, err := io.ReadAll(rc); err != nil || string(b) != voiceBytes {
		t.Fatalf("read = %q, %v", b, err)
	}
	if got := New(fakeToken, WithMaxSize(math.MaxInt64)).s.max; got != math.MaxInt64-1 {
		t.Errorf("max = %d, want clamp to MaxInt64-1", got)
	}
}

func TestCleanup(t *testing.T) {
	f := newFakeTG(t)
	dir := t.TempDir()

	t.Run("idempotent and returns nil", func(t *testing.T) {
		p, cleanup, err := f.dl().DownloadToTemp(context.Background(), fileID, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cleanup(); err != nil {
			t.Fatalf("first cleanup: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Fatalf("second cleanup: %v", err)
		}
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("file still there: %v", err)
		}
	})

	t.Run("never unlinks a replacement after cleanup", func(t *testing.T) {
		p, cleanup, err := f.dl().DownloadToTemp(context.Background(), fileID, dir)
		if err != nil {
			t.Fatal(err)
		}
		_ = cleanup()
		if err := os.WriteFile(p, []byte("someone else's"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "someone else's" {
			t.Errorf("replacement was removed: %q %v", b, err)
		}
	})

	t.Run("never unlinks a replacement swapped in before the first cleanup", func(t *testing.T) {
		p, cleanup, err := f.dl().DownloadToTemp(context.Background(), fileID, dir)
		if err != nil {
			t.Fatal(err)
		}
		other := filepath.Join(dir, "other")
		if err := os.WriteFile(other, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(other, p); err != nil { // the replacement coexisted with the original: distinct inode
			t.Fatal(err)
		}
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != "replacement" {
			t.Errorf("replacement was removed: %q %v", b, err)
		}
	})

	t.Run("file removed on panic", func(t *testing.T) {
		body := bodyFunc{
			read:  func([]byte) (int, error) { panic("boom") },
			close: func() error { return nil },
		}
		pdir := t.TempDir()
		d := New(fakeToken, WithBaseURL("http://127.0.0.1:1"), WithHTTPClient(scriptedDoer(body)))
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected the panic to propagate")
				}
			}()
			_, _, _ = d.DownloadToTemp(context.Background(), "f", pdir)
		}()
		if ents, _ := os.ReadDir(pdir); len(ents) != 0 {
			t.Errorf("temp file left behind after panic: %d", len(ents))
		}
	})
}

func TestNon200ReadCutShortKeepsContextClass(t *testing.T) {
	f := newFakeTG(t)
	f.download = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}
	_, _, err := f.dl(WithTimeout(100*time.Millisecond)).Download(context.Background(), fileID)
	var te *Error
	if !errors.As(err, &te) || te.StatusCode != http.StatusInternalServerError {
		t.Fatalf("err = %v, want *Error 500", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("lost DeadlineExceeded: %v", err)
	}
	noToken(t, err)
}
