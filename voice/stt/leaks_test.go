package stt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	fakeToken = "123456:SECRETbotTOKENvalue"
	fakeKey   = "sk-live-APIKEYvalue-7f3a"
)

func TestDownloadDoesNotSendAPIKey(t *testing.T) {
	var gotAuth atomic.Value
	gotAuth.Store("unset")
	dl := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(strings.Join(r.Header.Values("Authorization"), ","))
		_, _ = w.Write([]byte("ogg"))
	})
	c := New(dl, WithAPIKey(fakeKey), WithTempDir(t.TempDir()))
	p, err := c.downloadToTemp(context.Background(), dl+"/file/bot"+fakeToken+"/v.ogg")
	if err != nil {
		t.Fatal(err)
	}
	_ = p
	if got := gotAuth.Load(); got != "" {
		t.Errorf("download request carried Authorization %q", got)
	}

	// Positive control: the STT server itself still gets the key.
	var sttAuth atomic.Value
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		sttAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"text":"x"}`))
	})
	if _, err := New(srv, WithAPIKey(fakeKey)).Transcribe(context.Background(), audioFile(t)); err != nil {
		t.Fatal(err)
	}
	if sttAuth.Load() != "Bearer "+fakeKey {
		t.Errorf("STT request lost its key: %v", sttAuth.Load())
	}
}

func TestDownloadErrorsNeverContainURL(t *testing.T) {
	path := "/file/bot" + fakeToken + "/voice.ogg"
	slow := serve(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	notFound := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name   string
		client *Client
		ctx    context.Context
		url    string
		is     error
	}{
		{"connection refused", New(closedURL), context.Background(), closedURL + path, nil},
		{"timeout", New(slow, WithTimeout(50*time.Millisecond)), context.Background(), slow + path, context.DeadlineExceeded},
		{"HTTP 404", New(notFound), context.Background(), notFound + path, nil},
		{"canceled", New(slow), canceled, slow + path, context.Canceled},
		{"unparseable URL", New(slow), context.Background(), "http://bad host" + path, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.client.downloadToTemp(c.ctx, c.url)
			_, err2 := c.client.TranscribeURL(c.ctx, c.url)
			if err == nil || err2 == nil {
				t.Fatalf("want errors, got %v / %v", err, err2)
			}
			for _, e := range []error{err, err2} {
				var ue *url.Error
				if strings.Contains(e.Error(), "SECRET") || errors.As(e, &ue) {
					t.Errorf("error leaks the URL: %q (url.Error in chain: %v)", e, errors.As(e, &ue))
				}
				if c.is != nil && !errors.Is(e, c.is) {
					t.Errorf("error %q lost its class %v", e, c.is)
				}
			}
			if c.name == "HTTP 404" && !strings.Contains(err.Error(), "HTTP 404") {
				t.Errorf("status class missing: %v", err)
			}
		})
	}
}

func TestRequestErrorsNeverWrapURLError(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	base := closed.URL + "/bot" + fakeToken
	closed.Close()
	c := New(base)
	for _, kind := range []string{"transcribe", "verbose", "raw", "models"} {
		t.Run(kind, func(t *testing.T) {
			err := call(c, kind, audioFile(t))
			var ue *url.Error
			if err == nil || strings.Contains(err.Error(), "SECRET") || errors.As(err, &ue) {
				t.Errorf("err = %v (url.Error in chain: %v)", err, errors.As(err, &ue))
			}
		})
	}
	if _, err := New("http://bad host/bot" + fakeToken).Models(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("invalid base URL error leaks: %v", err)
	}
	if _, err := buildStreamURL("ht tp://x/bot"+fakeToken, StreamParams{}); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("invalid stream URL error leaks: %v", err)
	}
	if _, err := buildStreamURL("ftp://x/bot"+fakeToken, StreamParams{}); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("unsupported-scheme error leaks: %v", err)
	}
}

func TestDownloadRefusesRedirects(t *testing.T) {
	var hits atomic.Int32
	other := serve(t, func(http.ResponseWriter, *http.Request) { hits.Add(1) })
	src := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other, http.StatusFound) })
	_, err := New(src, WithAPIKey(fakeKey), WithTempDir(t.TempDir())).downloadToTemp(context.Background(), src+"/a.ogg")
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") || hits.Load() != 0 {
		t.Errorf("err=%v redirected-requests=%d, want HTTP 302 and no follow", err, hits.Load())
	}
}

func TestWithHTTPClientIsUsedForDownloadsAndNotMutated(t *testing.T) {
	var used atomic.Int32
	dl := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ogg")) })
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	c := New(dl, WithHTTPClient(hc), WithTimeout(7*time.Second), WithTempDir(t.TempDir()))
	if _, err := c.downloadToTemp(context.Background(), dl+"/a.ogg"); err != nil {
		t.Fatal(err)
	}
	if used.Load() != 1 || hc.Timeout != 0 || hc.CheckRedirect != nil {
		t.Errorf("used=%d caller client mutated (timeout=%v)", used.Load(), hc.Timeout)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReasonTokenIsStrictAndNeverTheAPIKey(t *testing.T) {
	cases := []struct {
		name, body, key, want string
	}{
		{"snake_case ok", `{"error":{"code":"invalid_api_key"}}`, "", "invalid_api_key"},
		{"UPPER ok", `{"error":{"type":"RATE_LIMITED"}}`, "", "RATE_LIMITED"},
		{"groq-shaped key", `{"error":{"code":"gsk_AbC123xyzSECRET"}}`, "", ""},
		{"openai-shaped key", `{"error":{"type":"sk-proj-abc123"}}`, "", ""},
		{"dash token", `{"error":{"code":"not-snake"}}`, "", ""},
		{"mixed case", `{"error":{"code":"InvalidApiKey"}}`, "", ""},
		{"reason equal to the client key", `{"error":{"code":"abcsecretkeyvalue"}}`, "abcsecretkeyvalue", ""},
		{"reason containing the client key", `{"error":{"code":"bad_abcsecretkeyvalue_x"}}`, "abcsecretkeyvalue", ""},
		{"unrelated key configured", `{"error":{"code":"invalid_api_key"}}`, "abcsecretkeyvalue", "invalid_api_key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := newHTTPError(http.StatusUnauthorized, []byte(c.body), c.key).Reason; got != c.want {
				t.Errorf("Reason = %q, want %q", got, c.want)
			}
		})
	}
}

func TestStreamKeyTravelsInHeaderOnly(t *testing.T) {
	var gotURL, gotAuth atomic.Value
	up := websocket.Upgrader{}
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		gotURL.Store(r.URL.String())
		gotAuth.Store(r.Header.Get("Authorization"))
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
	})
	cs, err := StreamWithChannelsAndAPIKey(context.Background(), srv, StreamParams{Language: "en"}, fakeKey)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.ForceClose()
	if u, _ := gotURL.Load().(string); strings.Contains(u, fakeKey) || strings.Contains(u, "sk-live") {
		t.Errorf("key in WebSocket URL: %s", u)
	}
	if gotAuth.Load() != "Bearer "+fakeKey {
		t.Errorf("Authorization = %v", gotAuth.Load())
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	_, err = StreamWithChannelsAndAPIKey(context.Background(), closedURL, StreamParams{}, fakeKey)
	if err == nil || strings.Contains(err.Error(), "APIKEYvalue") {
		t.Errorf("dial error: %v", err)
	}
}
