package stt

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// promptServer records the "prompt" form field of each transcription request.
func promptServer(t *testing.T, got *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("content type: %v", err)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		prompt, seen := "", false
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("multipart: %v", err)
				return
			}
			if part.FormName() == "prompt" {
				b, _ := io.ReadAll(part)
				prompt, seen = string(b), true
			}
		}
		if seen {
			*got = append(*got, prompt)
		} else {
			*got = append(*got, "<none>")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"ok"}`)
	}))
}

func TestWithPrompt_SendsThePromptField(t *testing.T) {
	var got []string
	srv := promptServer(t, &got)
	defer srv.Close()
	base := New(srv.URL)
	if _, err := base.With(WithPrompt("NVDA SGOV stop")).TranscribeReader(context.Background(), strings.NewReader("x"), "v.ogg"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.TranscribeReader(context.Background(), strings.NewReader("x"), "v.ogg"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "NVDA SGOV stop" {
		t.Fatalf("prompt fields = %q, want the prompt on the first call", got)
	}
	if got[1] != "<none>" {
		t.Fatalf("With mutated the original client: second call sent %q", got[1])
	}
}
