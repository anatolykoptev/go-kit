package tgsafe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const (
	token   = "123456789:AAFakeTokenFakeTokenFake_x-y"
	oddKey  = "k3y/with+odd=chars"
	botPath = "/bot" + token + "/getUpdates"
)

func TestScrub(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"api url", "Post https://api.telegram.org" + botPath, "Post https://api.telegram.org/bot<redacted>/getUpdates"},
		{"file url", "https://api.telegram.org/file/bot" + token + "/voice/a.oga", "https://api.telegram.org/file/bot<redacted>/voice/a.oga"},
		{"two tokens", "bot111111:AAAAAAAAAAAAAAAAAAAAA and bot222222:BBBBBBBBBBBBBBBBBBBBB", "bot<redacted> and bot<redacted>"},
		{"nothing to scrub", "dial tcp: connection refused", "dial tcp: connection refused"},
	}
	for _, c := range cases {
		if got := Scrub(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestScrubLiteralSecretsRawAndEscaped(t *testing.T) {
	in := "key=" + url.QueryEscape(oddKey) + " raw=" + oddKey
	got := Scrub(in, oddKey, "")
	if strings.Contains(got, "k3y") || strings.Count(got, Placeholder) != 2 {
		t.Errorf("got %q", got)
	}
	// A token that does not look like bot<id>:<secret> is still removed when given as a secret.
	if got := Scrub("path /bot-short-token/x", "short-token"); strings.Contains(got, "short-token") {
		t.Errorf("literal secret survived: %q", got)
	}
}

func TestErrorKeepsURLErrorTypeAndTimeout(t *testing.T) {
	orig := &url.Error{Op: "Post", URL: "https://api.telegram.org" + botPath, Err: timeoutErr{}}
	got := Error(orig)
	var ue *url.Error
	if !errors.As(got, &ue) {
		t.Fatalf("lost *url.Error: %T", got)
	}
	if !ue.Timeout() {
		t.Error("Timeout() lost")
	}
	if strings.Contains(got.Error(), "AAFake") {
		t.Errorf("token in %q", got)
	}
	if !strings.Contains(orig.Error(), "AAFake") {
		t.Error("input was mutated")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestErrorScrubsWrappedCause(t *testing.T) {
	inner := fmt.Errorf("redirect to https://x%s failed", botPath)
	got := Error(&url.Error{Op: "Get", URL: "https://x/", Err: inner})
	if strings.Contains(got.Error(), "AAFake") {
		t.Errorf("token in wrapped cause: %q", got)
	}
}

func TestErrorCutsChainForOtherErrorsCarryingASecret(t *testing.T) {
	leaky := fmt.Errorf("send: %w", &url.Error{Op: "Post", URL: botPath, Err: errors.New("boom")})
	got := Error(leaky)
	var ue *url.Error
	if strings.Contains(got.Error(), "AAFake") || errors.As(got, &ue) {
		t.Errorf("token or original chain still reachable: %q", got)
	}
	clean := errors.New("fine")
	if !errors.Is(Error(clean), clean) || Error(nil) != nil {
		t.Error("clean/nil errors must pass through unchanged")
	}
}

func TestHTTPClientScrubsRealTransportFailure(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens: connection refused, a real *url.Error from net/http
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+addr+botPath, nil)

	resp, rawErr := (&http.Client{}).Do(req)
	closeBody(resp)
	if rawErr == nil || !strings.Contains(rawErr.Error(), "AAFake") {
		t.Fatalf("control: the plain client should leak the token, got %v", rawErr)
	}
	resp, err = NewHTTPClient(nil, token).Do(req)
	closeBody(resp)
	var ue *url.Error
	if err == nil || strings.Contains(err.Error(), "AAFake") || !errors.As(err, &ue) {
		t.Errorf("scrubbing client: err=%v (url.Error kept: %v)", err, errors.As(err, &ue))
	}
}

func TestLoggerScrubsPrintlnAndPrintf(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(slog.New(slog.NewTextHandler(&buf, nil)), token, oddKey)
	l.Println(&url.Error{Op: "Post", URL: "https://api.telegram.org" + botPath, Err: errors.New("boom")})
	l.Printf("Endpoint: %s, key %s", "https://x"+botPath, oddKey)
	out := buf.String()
	if strings.Contains(out, "AAFake") || strings.Contains(out, token) || strings.Contains(out, "k3y") {
		t.Errorf("logger leaks: %s", out)
	}
	if !strings.Contains(out, "bot<redacted>") || !strings.Contains(out, "boom") {
		t.Errorf("logger lost the useful part: %s", out)
	}
}

func TestErrorAttr(t *testing.T) {
	if !ErrorAttr(nil).Equal(slog.Attr{}) {
		t.Error("nil error must give the zero Attr")
	}
	a := ErrorAttr(fmt.Errorf("get: %w", &url.Error{Op: "Get", URL: "https://api.telegram.org/file" + botPath, Err: errors.New("eof")}), oddKey)
	if a.Key != "error" || strings.Contains(a.Value.String(), "AAFake") || !strings.Contains(a.Value.String(), "eof") {
		t.Errorf("attr = %v", a)
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("x", a)
	if strings.Contains(buf.String(), "AAFake") {
		t.Errorf("rendered: %s", buf.String())
	}
}

func closeBody(r *http.Response) {
	if r != nil {
		r.Body.Close()
	}
}
