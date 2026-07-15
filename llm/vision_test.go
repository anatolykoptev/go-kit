package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestDataURI(t *testing.T) {
	got := llm.DataURI("image/png", []byte{1, 2, 3})
	want := "data:image/png;base64,AQID"
	if got != want {
		t.Errorf("DataURI = %q, want %q", got, want)
	}
}

func TestCompleteMultimodal_Base64Detail(t *testing.T) {
	var capturedBody []byte
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})
	c := llm.NewClient(srv.URL, "key", "model")

	_, err := c.CompleteMultimodal(context.Background(), "describe", []llm.ImagePart{
		{Base64: "AQID", MIMEType: "image/png", Detail: "high"},
	})
	if err != nil {
		t.Fatalf("CompleteMultimodal: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}

	msgs := req["messages"].([]any)
	userMsg := msgs[0].(map[string]any)
	parts := userMsg["content"].([]any)

	// parts[0] = text, parts[1] = image
	if len(parts) != 2 {
		t.Fatalf("content parts len = %d, want 2", len(parts))
	}

	imgPart := parts[1].(map[string]any)
	if imgPart["type"] != "image_url" {
		t.Errorf("image part type = %q, want image_url", imgPart["type"])
	}

	imageURL := imgPart["image_url"].(map[string]any)
	wantURL := "data:image/png;base64,AQID"
	if imageURL["url"] != wantURL {
		t.Errorf("image_url.url = %q, want %q", imageURL["url"], wantURL)
	}
	if imageURL["detail"] != "high" {
		t.Errorf("image_url.detail = %q, want high", imageURL["detail"])
	}
}

func TestCompleteMultimodal_URLDetail(t *testing.T) {
	var capturedBody []byte
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "ok"}},
			},
		})
	})
	c := llm.NewClient(srv.URL, "key", "model")

	_, err := c.CompleteMultimodal(context.Background(), "describe", []llm.ImagePart{
		{URL: "https://example.com/img.png", Detail: "low"},
	})
	if err != nil {
		t.Fatalf("CompleteMultimodal: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}

	msgs := req["messages"].([]any)
	userMsg := msgs[0].(map[string]any)
	parts := userMsg["content"].([]any)
	imgPart := parts[1].(map[string]any)
	imageURL := imgPart["image_url"].(map[string]any)

	if imageURL["url"] != "https://example.com/img.png" {
		t.Errorf("url = %q, want original URL", imageURL["url"])
	}
	if imageURL["detail"] != "low" {
		t.Errorf("detail = %q, want low", imageURL["detail"])
	}
}
