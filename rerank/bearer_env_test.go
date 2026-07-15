package rerank_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/rerank"
)

func TestNewClientBearerEnvAutoResolve(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"index": 0, "relevance_score": 0.5}},
		})
	}))
	defer srv.Close()

	t.Run("EMBED_TOKEN env applied as apiKey via v2 NewClient", func(t *testing.T) {
		capturedAuth = ""
		t.Setenv("EMBED_TOKEN", "rerank-env-token")
		c := rerank.NewClient(srv.URL, rerank.WithModel("r"))
		_ = c.Rerank(context.Background(), "q", []rerank.Doc{{Text: "a"}, {Text: "b"}})
		if capturedAuth != "Bearer rerank-env-token" {
			t.Errorf("Authorization = %q, want Bearer rerank-env-token", capturedAuth)
		}
	})

	t.Run("explicit WithAPIKey overrides env", func(t *testing.T) {
		capturedAuth = ""
		t.Setenv("EMBED_TOKEN", "env-token")
		c := rerank.NewClient(srv.URL, rerank.WithModel("r"), rerank.WithAPIKey("opt-token"))
		_ = c.Rerank(context.Background(), "q", []rerank.Doc{{Text: "a"}})
		if capturedAuth != "Bearer opt-token" {
			t.Errorf("Authorization = %q, want Bearer opt-token (explicit > env)", capturedAuth)
		}
	})
}
