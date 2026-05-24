package sparse_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/sparse"
)

func TestNewClientBearerEnvAutoResolve(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"indices": []int{1}, "values": []float32{0.5}}},
		})
	}))
	defer srv.Close()

	t.Run("EMBED_TOKEN env applied via v2 NewClient", func(t *testing.T) {
		capturedAuth = ""
		t.Setenv("EMBED_TOKEN", "sparse-env-token")
		c, err := sparse.NewClient(srv.URL, sparse.WithModel("m"))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = c.EmbedSparse(context.Background(), []string{"x"})
		if err != nil {
			t.Fatalf("EmbedSparse: %v", err)
		}
		if capturedAuth != "Bearer sparse-env-token" {
			t.Errorf("Authorization = %q, want Bearer sparse-env-token", capturedAuth)
		}
	})
}
