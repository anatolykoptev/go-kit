package voice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFallback_GeminiFailuresUseEdge(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		client  func(*httptest.Server) *http.Client
	}{
		{"429", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }, nil},
		{"500", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, nil},
		{"timeout", func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}, func(s *httptest.Server) *http.Client { c := s.Client(); c.Timeout = 50 * time.Millisecond; return c }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			enc := &encoderLog{}
			g := newTestGemini(t, srv, testKey, enc)
			if c.client != nil {
				g.client = c.client(srv)
			}
			edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
			log, buf := captureLog()

			got, err := NewFallbackProvider("gemini", g, edge, log).Synthesize(context.Background(), secretText, "ru")
			if err != nil || got != "/tmp/edge.ogg" || edge.calls.Load() != 1 {
				t.Fatalf("got %q err=%v edgeCalls=%d", got, err, edge.calls.Load())
			}
			if enc.len() != 0 {
				t.Error("gemini must not encode on failure")
			}
			dump := buf.String()
			if !strings.Contains(dump, "falling back") {
				t.Errorf("fallback not logged: %s", dump)
			}
			if strings.Contains(dump, secretText) || strings.Contains(dump, testKey) {
				t.Errorf("log leaks text or key: %s", dump)
			}
		})
	}
}

func TestFallback_MissingKeyUsesEdgeWithoutRequest(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reqs.Add(1) }))
	defer srv.Close()
	enc := &encoderLog{}
	g := newTestGemini(t, srv, "", enc)
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	fb := NewFallbackProvider("gemini", g, edge, nil)

	got, err := fb.Synthesize(context.Background(), "hi", "")
	if err != nil || got != "/tmp/edge.ogg" || reqs.Load() != 0 {
		t.Fatalf("got %q err=%v requests=%d", got, err, reqs.Load())
	}
	if _, err := g.Synthesize(context.Background(), "hi", ""); err == nil || reqs.Load() != 0 {
		t.Errorf("direct gemini call without key: err=%v requests=%d", err, reqs.Load())
	}
}

func TestFallback_CallerCancelDoesNotStartEdge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(500 * time.Millisecond) }))
	defer srv.Close()
	enc := &encoderLog{}
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	log, buf := captureLog()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)
	if _, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), edge, log).Synthesize(ctx, "hi", ""); err == nil {
		t.Fatal("want error")
	}
	if edge.calls.Load() != 0 {
		t.Error("edge must not run after the caller's context ended")
	}
	if !strings.Contains(buf.String(), "canceled") {
		t.Errorf("skipped fallback not logged: %s", buf.String())
	}
}

type slowFailProvider struct{ wait time.Duration }

func (s *slowFailProvider) Synthesize(context.Context, string, string) (string, error) {
	time.Sleep(s.wait)
	return "", errors.New("slow failure")
}
func (s *slowFailProvider) IsAvailable() bool { return true }

func TestFallback_ParentDeadlineSkipsEdgeAndLogs(t *testing.T) {
	// Drive the composite with a primary that returns after the parent
	// deadline passed.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	primary := &slowFailProvider{wait: 60 * time.Millisecond}
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	log, buf := captureLog()
	if _, err := NewFallbackProvider("gemini", primary, edge, log).Synthesize(ctx, "hi", ""); err == nil {
		t.Fatal("want error")
	}
	if edge.calls.Load() != 0 {
		t.Error("edge must not run after the parent deadline")
	}
	if !strings.Contains(buf.String(), "parent_deadline") {
		t.Errorf("skip not logged as parent_deadline: %s", buf.String())
	}
}

func TestFallback_IsAvailableReflectsEitherProvider(t *testing.T) {
	cases := []struct {
		key        string
		ffmpeg     bool
		edge, want bool
	}{
		{testKey, true, false, true}, // gemini alone
		{"", true, true, true},       // no key, edge alone
		{testKey, false, true, true}, // no ffmpeg for gemini, edge alone
		{testKey, false, false, false},
		{"", true, false, false},
	}
	for _, c := range cases {
		g := NewGeminiProvider(GeminiConfig{APIKey: c.key})
		g.ffmpegOK = func() bool { return c.ffmpeg }
		fb := NewFallbackProvider("gemini", g, &stubProvider{avail: c.edge}, nil)
		if got := fb.IsAvailable(); got != c.want {
			t.Errorf("%+v: IsAvailable = %v, want %v", c, got, c.want)
		}
	}
}

func TestNewProviderFromSettings(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
		want string
	}{
		{"empty is edge", Settings{}, "edge"},
		{"explicit edge", Settings{Provider: "edge"}, "edge"},
		{"unknown provider falls to edge", Settings{Provider: "bogus"}, "edge"},
		{"gemini wraps with fallback", Settings{Provider: "Gemini", GeminiAPIKey: testKey}, "fallback"},
		{"gemini without key still wrapped", Settings{Provider: "gemini"}, "fallback"},
		{"fallback none is bare gemini", Settings{Provider: "gemini", Fallback: "none", GeminiAPIKey: testKey}, "gemini"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := providerKind(NewProviderFromSettings(c.s)); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func providerKind(v Provider) string {
	switch v.(type) {
	case *EdgeProvider:
		return "edge"
	case *FallbackProvider:
		return "fallback"
	case *GeminiProvider:
		return "gemini"
	}
	return "other"
}

func TestFallback_SlowGeminiLeavesEdgeItsReserve(t *testing.T) {
	// Gemini would answer after 6 s; the caller's deadline leaves it only
	// ~3.5 s after the edge reserve, so Gemini must be cut off and edge must
	// still run inside the caller's remaining time.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(6 * time.Second):
			reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
		}
	}))
	defer srv.Close()
	enc := &encoderLog{}
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	ctx, cancel := context.WithTimeout(context.Background(), EdgeReserve+3500*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), edge, nil).Synthesize(ctx, "hi", "")
	if err != nil || got != "/tmp/edge.ogg" || edge.calls.Load() != 1 {
		t.Fatalf("got %q err=%v edgeCalls=%d", got, err, edge.calls.Load())
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Errorf("gemini was not cut off at its budget: %v", el)
	}
	if ctx.Err() != nil {
		t.Error("caller context must still be alive for edge")
	}
}

func TestFallback_NoBudgetSkipsGeminiRequest(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reqs.Add(1) }))
	defer srv.Close()
	enc := &encoderLog{}
	g := newTestGemini(t, srv, testKey, enc)
	short, cancelShort := context.WithTimeout(context.Background(), time.Second)
	defer cancelShort()
	if _, err := g.Synthesize(short, "hi", ""); !errors.Is(err, ErrBudgetExhausted) || reqs.Load() != 0 {
		t.Errorf("bare, <3s left: err=%v requests=%d", err, reqs.Load())
	}
	// Through the fallback only reserve+1s remains, i.e. 1s for Gemini.
	ctx, cancel := context.WithTimeout(context.Background(), EdgeReserve+time.Second)
	defer cancel()
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	if got, err := NewFallbackProvider("gemini", g, edge, nil).Synthesize(ctx, "hi", ""); err != nil || got != "/tmp/edge.ogg" || reqs.Load() != 0 {
		t.Errorf("fallback: got %q err=%v requests=%d", got, err, reqs.Load())
	}
}

func TestFallback_UnavailableEdgeReturnsGeminiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }))
	defer srv.Close()
	enc := &encoderLog{}
	edge := &stubProvider{avail: false}
	_, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), edge, nil).Synthesize(context.Background(), "hi", "")
	var he *GeminiHTTPError
	if !errors.As(err, &he) || he.Status != 429 || edge.calls.Load() != 0 {
		t.Errorf("err=%v edgeCalls=%d, want the gemini 429 and no edge call", err, edge.calls.Load())
	}
}

func TestFallback_BothFailJoinsErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer srv.Close()
	enc := &encoderLog{}
	edgeErr := errors.New("edge boom")
	_, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), &stubProvider{avail: true, err: edgeErr}, nil).
		Synthesize(context.Background(), "hi", "")
	var he *GeminiHTTPError
	if !errors.Is(err, edgeErr) || !errors.As(err, &he) || he.Status != 503 {
		t.Errorf("err = %v, want both gemini 503 and edge error", err)
	}
}
