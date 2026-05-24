package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/embed"
)

func TestHTTPEmbedderBearerToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1, 0.2}, "index": 0}},
		})
	}))
	defer srv.Close()

	t.Run("WithBearerToken sets Authorization", func(t *testing.T) {
		capturedAuth = ""
		c := embed.NewHTTPEmbedder(srv.URL, "test-model", 2, nil, embed.WithBearerToken("secret-token"))
		_, err := c.Embed(context.Background(), []string{"hello"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if capturedAuth != "Bearer secret-token" {
			t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer secret-token")
		}
	})

	t.Run("no token = no Authorization header", func(t *testing.T) {
		capturedAuth = ""
		c := embed.NewHTTPEmbedder(srv.URL, "test-model", 2, nil)
		_, err := c.Embed(context.Background(), []string{"hello"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if capturedAuth != "" {
			t.Errorf("Authorization unexpectedly set = %q", capturedAuth)
		}
	})

	t.Run("empty token = no Authorization header", func(t *testing.T) {
		capturedAuth = ""
		c := embed.NewHTTPEmbedder(srv.URL, "test-model", 2, nil, embed.WithBearerToken(""))
		_, err := c.Embed(context.Background(), []string{"hello"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if capturedAuth != "" {
			t.Errorf("empty token should not set header, got %q", capturedAuth)
		}
	})
}
