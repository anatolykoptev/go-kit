package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestIsAvailableHealthEndpoint verifies that IsAvailable returns true
// when /health responds with 200 (self-hosted server case).
func TestIsAvailableHealthEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := stt.New(srv.URL)
	if !c.IsAvailable() {
		t.Error("IsAvailable should return true when /health responds 200")
	}
}

// TestIsAvailableModelsFallback verifies that IsAvailable falls back to
// /v1/models when /health is not available (Groq, OpenAI case).
func TestIsAvailableModelsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := stt.New(srv.URL)
	if !c.IsAvailable() {
		t.Error("IsAvailable should fall back to /v1/models when /health is 404")
	}
}

// TestIsAvailableBothFail verifies that IsAvailable returns false when
// neither /health nor /v1/models respond.
func TestIsAvailableBothFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := stt.New(srv.URL)
	if c.IsAvailable() {
		t.Error("IsAvailable should return false when both endpoints fail")
	}
}

// TestIsAvailableWithAPIKey verifies that IsAvailable sends the API key
// header on both probe requests (needed for Groq/OpenAI /v1/models).
func TestIsAvailableWithAPIKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := stt.New(srv.URL, stt.WithAPIKey("test-key-123"))
	if !c.IsAvailable() {
		t.Fatal("IsAvailable should return true via /v1/models")
	}
	if gotAuth != "Bearer test-key-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key-123")
	}
}

// TestTranscribeEmptyLanguageNotSent verifies that when language is empty,
// the "language" field is NOT sent in the multipart body (lets the STT
// service auto-detect — needed for Groq/OpenAI auto-detect mode).
func TestTranscribeEmptyLanguageNotSent(t *testing.T) {
	var gotFields map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotFields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			if len(v) > 0 {
				gotFields[k] = v[0]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	}))
	defer srv.Close()

	// Create a temp audio file.
	tmp := t.TempDir() + "/test.wav"
	if err := os.WriteFile(tmp, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	// language="" → should NOT send language field
	c := stt.New(srv.URL, stt.WithLanguage(""))
	resp, err := c.Transcribe(context.Background(), tmp)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if resp.Text != "hello" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello")
	}
	if _, ok := gotFields["language"]; ok {
		t.Error("language field should NOT be sent when language is empty")
	}
}
