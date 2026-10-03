package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestAPIKeySentInAuthorization verifies that WithAPIKey sets the
// Authorization: Bearer header on HTTP requests to the transcription endpoint.
func TestAPIKeySentInAuthorization(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello","language":"en"}`))
	}))
	defer srv.Close()

	c := stt.New(srv.URL, stt.WithAPIKey("sk-test-key-789"))
	if _, err := c.TranscribeReader(context.Background(), strings.NewReader("fake audio"), "test.wav"); err != nil {
		t.Fatalf("TranscribeReader: %v", err)
	}
	if gotAuth != "Bearer sk-test-key-789" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk-test-key-789")
	}
}

// TestAPIKeyNotSetWhenEmpty verifies that no Authorization header is sent
// when WithAPIKey is not used (backward compat with self-hosted server).
func TestAPIKeyNotSetWhenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello","language":"en"}`))
	}))
	defer srv.Close()

	c := stt.New(srv.URL)
	if _, err := c.TranscribeReader(context.Background(), strings.NewReader("fake audio"), "test.wav"); err != nil {
		t.Fatalf("TranscribeReader: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (no API key configured)", gotAuth)
	}
}

// TestAPIKeySentInModels verifies that WithAPIKey sets the Authorization
// header on GET /v1/models requests too.
func TestAPIKeySentInModels(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := stt.New(srv.URL, stt.WithAPIKey("sk-models-key"))
	if _, err := c.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	if gotAuth != "Bearer sk-models-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer sk-models-key")
	}
}
