package sparse_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/sparse"
)

func TestHTTPSparseBearerToken(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"indices": []int{1, 2}, "values": []float32{0.5, 0.3}}},
		})
	}))
	defer srv.Close()

	t.Run("WithBearerToken sets Authorization", func(t *testing.T) {
		capturedAuth = ""
		c := sparse.NewHTTPSparseEmbedder(srv.URL, "test-splade", nil, sparse.WithBearerToken("secret"))
		_, err := c.EmbedSparse(context.Background(), []string{"x"})
		if err != nil {
			t.Fatalf("SparseEmbed: %v", err)
		}
		if capturedAuth != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", capturedAuth)
		}
	})

	t.Run("no token", func(t *testing.T) {
		capturedAuth = ""
		c := sparse.NewHTTPSparseEmbedder(srv.URL, "test-splade", nil)
		_, err := c.EmbedSparse(context.Background(), []string{"x"})
		if err != nil {
			t.Fatalf("SparseEmbed: %v", err)
		}
		if capturedAuth != "" {
			t.Errorf("unexpected Authorization=%q", capturedAuth)
		}
	})
}
