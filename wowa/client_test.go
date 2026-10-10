package wowa

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// jsonBody writes v as a JSON response with the given status code.
func jsonBody(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func TestRenderDecodesAndSendsSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/render" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Internal-Secret"); got != "s3cr3t" {
			t.Errorf("X-Internal-Secret = %q, want s3cr3t", got)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["url"] != "https://example.com" {
			t.Errorf("url = %v", req["url"])
		}
		if req["timeout_secs"] != float64(15) {
			t.Errorf("timeout_secs = %v, want 15", req["timeout_secs"])
		}
		if req["wait"] != "networkidle" {
			t.Errorf("wait = %v", req["wait"])
		}
		jsonBody(w, http.StatusOK, map[string]any{
			"url": "https://example.com", "html": "<html><body>hi</body></html>",
			"title": "Example", "status": 200, "elapsed_ms": 812,
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("s3cr3t"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Render(context.Background(), RenderRequest{
		URL: "https://example.com", TimeoutSecs: 15, Wait: "networkidle",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.HTML != "<html><body>hi</body></html>" || resp.Title != "Example" || resp.ElapsedMs != 812 {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestRenderHTMLErrorFieldIn200(t *testing.T) {
	// go-wowa answers HTTP 200 on render failure — the error field is the
	// signal. Literal wire JSON so a struct-tag typo can't round-trip.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"url":"https://x","html":"","status":0,"elapsed_ms":20001,"error":"nav timeout"}`)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.RenderHTML(context.Background(), "https://x", 20, "")
	var re *RemoteError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v, want RemoteError", err)
	}
	if re.StatusCode != 200 || re.Message != "nav timeout" {
		t.Fatalf("RemoteError = %+v", re)
	}
}

func TestRenderEmptyHTMLIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonBody(w, http.StatusOK, map[string]any{"url": "https://x", "status": 200})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Render(context.Background(), RenderRequest{URL: "https://x"}); err == nil {
		t.Fatal("empty html accepted — convention removed")
	}
}

func TestFetchUsesCanonicalTimeoutField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/fetch" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["timeout"] != float64(8) {
			t.Errorf("timeout = %v, want 8 (canonical ox field)", req["timeout"])
		}
		if _, ok := req["timeout_secs"]; ok {
			t.Errorf("legacy timeout_secs must not be sent on /fetch")
		}
		jsonBody(w, http.StatusOK, map[string]any{
			"status": 404, "body": "not found page", "headers": map[string]string{},
			"cf_detected": false, "elapsed_ms": 120,
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// Upstream 404 is data, not a client error.
	resp, err := c.Fetch(context.Background(), FetchRequest{URL: "https://x", TimeoutSecs: 8})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.Status != 404 || resp.Body != "not found page" {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestFetch502EnvelopeIsRemoteError(t *testing.T) {
	// ox /fetch answers 502 with a FetchResponse body carrying the error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonBody(w, http.StatusBadGateway, map[string]any{
			"status": 0, "body": "", "headers": map[string]string{},
			"cf_detected": false, "elapsed_ms": 8000, "error": "deadline exceeded (8s per-call bound)",
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var re *RemoteError
	if _, err := c.Fetch(context.Background(), FetchRequest{URL: "https://x"}); !errors.As(err, &re) {
		t.Fatalf("err = %v, want RemoteError", err)
	}
	if re.StatusCode != 502 || re.Message != "deadline exceeded (8s per-call bound)" {
		t.Fatalf("RemoteError = %+v", re)
	}
}

func TestReadDecodesContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/read" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["max_length"] != float64(5000) {
			t.Errorf("max_length = %v", req["max_length"])
		}
		jsonBody(w, http.StatusOK, map[string]any{
			"title": "T", "content": "article text", "url": "https://x",
			"format": "text", "length": 12, "method": "readability",
			"elapsed_ms": 300, "site_name": "X", "language": "en",
			"extraction_note": "extraction_rejected_low_text_ratio",
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Read(context.Background(), ReadRequest{URL: "https://x", MaxLength: 5000})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if resp.Content != "article text" || resp.ExtractionNote != "extraction_rejected_low_text_ratio" {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestInteractDecodesActionResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chrome/interact" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["session"] != "sess1" || req["mode"] != "default" {
			t.Errorf("session/mode = %v/%v", req["session"], req["mode"])
		}
		actions, _ := req["actions"].([]any)
		if len(actions) != 1 || actions[0].(map[string]any)["type"] != "evaluate" {
			t.Errorf("actions = %v", req["actions"])
		}
		jsonBody(w, http.StatusOK, map[string]any{
			"url": "https://x", "status": "ok", "session_id": "sess1", "elapsed_ms": 55,
			"actions": []map[string]any{
				{"action": "evaluate", "ok": true, "data": map[string]any{"status": 200, "body": "hi"}},
			},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Interact(context.Background(), InteractRequest{
		URL:     "https://x",
		Session: "sess1",
		Mode:    "default",
		Actions: []Action{{Type: "evaluate", Script: "1+1"}},
	})
	if err != nil {
		t.Fatalf("Interact: %v", err)
	}
	if len(resp.Actions) != 1 || !resp.Actions[0].Ok {
		t.Fatalf("actions = %+v", resp.Actions)
	}
	var data struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(resp.Actions[0].Data, &data); err != nil || data.Body != "hi" {
		t.Fatalf("action data = %s, err %v", resp.Actions[0].Data, err)
	}
}

func TestInteractStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonBody(w, http.StatusOK, map[string]any{
			"url": "https://x", "status": "error",
			"error": "context pool not available", "error_code": "chrome_unavailable",
			"actions": []map[string]any{},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var re *RemoteError
	if _, err := c.Interact(context.Background(), InteractRequest{
		URL: "https://x", Actions: []Action{{Type: "evaluate", Script: "1"}},
	}); !errors.As(err, &re) {
		t.Fatalf("err = %v, want RemoteError", err)
	}
	if re.Message != "context pool not available" {
		t.Fatalf("message = %q", re.Message)
	}
}

func TestExtractRejectsOnErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/extract" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		jsonBody(w, http.StatusBadRequest, map[string]string{"error": "url and prompt are required"})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var re *RemoteError
	if _, err := c.Extract(context.Background(), ExtractRequest{}); !errors.As(err, &re) {
		t.Fatalf("err = %v, want RemoteError", err)
	}
	if re.StatusCode != 400 || re.Message != "url and prompt are required" {
		t.Fatalf("RemoteError = %+v", re)
	}
}

func TestExtractSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonBody(w, http.StatusOK, map[string]any{
			"data": map[string]any{"price": "9.99"}, "source_method": "read",
			"chars_used": 4200, "attempts": 1,
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Extract(context.Background(), ExtractRequest{URL: "https://x", Prompt: "get price"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	var data map[string]string
	if err := json.Unmarshal(resp.Data, &data); err != nil || data["price"] != "9.99" {
		t.Fatalf("data = %s, err %v", resp.Data, err)
	}
}

func TestNonEnvelopeErrorIsStatusError(t *testing.T) {
	// A proxy-level failure returns a non-JSON body — no error field to
	// surface, so it must be a StatusError, not a RemoteError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "upstream connect failed")
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var se *StatusError
	if _, err := c.Render(context.Background(), RenderRequest{URL: "https://x"}); !errors.As(err, &se) {
		t.Fatalf("err = %v, want StatusError", err)
	}
	if se.StatusCode != 503 || !strings.Contains(se.Body, "upstream") {
		t.Fatalf("StatusError = %+v", se)
	}
}

func TestClientTimeoutIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		jsonBody(w, http.StatusOK, map[string]any{"url": "x"})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"), WithTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var te *TimeoutError
	if _, err := c.Render(context.Background(), RenderRequest{URL: "https://x"}); !errors.As(err, &te) {
		t.Fatalf("err = %v, want TimeoutError", err)
	}
}

func TestTruncatedBodyIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"html":"`+strings.Repeat("x", maxBodyBytes)+`"}`)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Render(context.Background(), RenderRequest{URL: "https://x"}); !errors.Is(err, ErrBodyTruncated) {
		t.Fatalf("err = %v, want ErrBodyTruncated", err)
	}
}

func TestRequireAuthFailsConstruction(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "")
	if _, err := NewClient("http://127.0.0.1:1", WithRequireAuth()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestEnvSecretResolution(t *testing.T) {
	t.Setenv("INTERNAL_SERVICE_SECRET", "envsecret")
	c, err := NewClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.secret != "envsecret" {
		t.Fatalf("secret = %q, want envsecret", c.secret)
	}
}

func TestNoSecretSendsNoHeader(t *testing.T) {
	// SOFT-auth deployment: no secret configured must NOT panic and must not
	// send the header — go-wowa's soft middleware allows absent headers.
	t.Setenv("INTERNAL_SERVICE_SECRET", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Internal-Secret"); got != "" {
			t.Errorf("X-Internal-Secret = %q, want absent", got)
		}
		jsonBody(w, http.StatusOK, map[string]any{"url": "x", "html": "<h1/>", "status": 200})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.RenderHTML(context.Background(), "https://x", 10, ""); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
}

func TestPingReachability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		w.WriteHeader(http.StatusNotFound) // any HTTP response = reachable
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("k"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	srv.Close()
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("Ping to dead server: want error")
	}
}
