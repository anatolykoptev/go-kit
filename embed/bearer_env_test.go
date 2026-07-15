package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/embed"
)

func TestNewClientBearerEnvAutoResolve(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.1}, "index": 0}},
		})
	}))
	defer srv.Close()

	t.Run("EMBED_TOKEN env auto-applied through v2 NewClient", func(t *testing.T) {
		capturedAuth = ""
		t.Setenv("EMBED_TOKEN", "env-token-789")
		c, err := embed.NewClient(srv.URL, embed.WithBackend("http"), embed.WithDim(1))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = c.Embed(context.Background(), []string{"x"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if capturedAuth != "Bearer env-token-789" {
			t.Errorf("Authorization = %q, want Bearer env-token-789", capturedAuth)
		}
	})

	t.Run("no env = no Authorization header", func(t *testing.T) {
		capturedAuth = ""
		t.Setenv("EMBED_TOKEN", "")
		c, err := embed.NewClient(srv.URL, embed.WithBackend("http"), embed.WithDim(1))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = c.Embed(context.Background(), []string{"x"})
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if capturedAuth != "" {
			t.Errorf("unexpected Authorization=%q", capturedAuth)
		}
	})
}
