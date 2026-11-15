package stt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestTranscribeVerbose(t *testing.T) {
	want := stt.VerboseResponse{
		Text: "hello world", Language: "en", Duration: 3.14, LanguageConfidence: 0.99,
		Segments: []stt.Segment{{ID: 0, Start: 0.0, End: 1.5, Text: "hello"}, {ID: 1, Start: 1.5, End: 3.14, Text: "world"}},
		Words:    []stt.Word{{Word: "hello", Start: 0.0, End: 0.8, Speaker: "A"}, {Word: "world", Start: 1.5, End: 2.2, Speaker: "A"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		if got := r.FormValue("response_format"); got != "verbose_json" {
			t.Errorf("response_format = %q, want verbose_json", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("fake audio"), 0o644)

	resp, err := stt.New(srv.URL, stt.WithLanguage("en")).TranscribeVerbose(context.Background(), tmp)
	if err != nil {
		t.Fatalf("TranscribeVerbose: %v", err)
	}
	if resp.Text != want.Text {
		t.Errorf("text = %q, want %q", resp.Text, want.Text)
	}
	if len(resp.Segments) != 2 || resp.Segments[0].Text != "hello" {
		t.Errorf("segments mismatch: %v", resp.Segments)
	}
	if len(resp.Words) != 2 || resp.Words[1].Word != "world" {
		t.Errorf("words mismatch: %v", resp.Words)
	}
	if resp.LanguageConfidence != 0.99 {
		t.Errorf("language_confidence = %v, want 0.99", resp.LanguageConfidence)
	}
}

func TestTranscribeRaw(t *testing.T) {
	srtContent := "1\n00:00:00,000 --> 00:00:01,500\nhello world\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(srtContent))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.mp3")
	_ = os.WriteFile(tmp, []byte("fake audio"), 0o644)

	raw, err := stt.New(srv.URL, stt.WithFormat("srt")).TranscribeRaw(context.Background(), tmp)
	if err != nil {
		t.Fatalf("TranscribeRaw: %v", err)
	}
	if string(raw) != srtContent {
		t.Errorf("raw = %q, want %q", string(raw), srtContent)
	}
}

func TestTranscribeReader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		_, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		if header.Filename != "stream.wav" {
			t.Errorf("filename = %q, want stream.wav", header.Filename)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stt.Response{Text: "streamed", Language: "ru"})
	}))
	defer srv.Close()

	resp, err := stt.New(srv.URL).TranscribeReader(context.Background(), bytes.NewReader([]byte("audio")), "stream.wav")
	if err != nil {
		t.Fatalf("TranscribeReader: %v", err)
	}
	if resp.Text != "streamed" {
		t.Errorf("text = %q, want streamed", resp.Text)
	}
}

func TestModels(t *testing.T) {
	want := stt.ModelList{Object: "list", Data: []stt.Model{
		{ID: "moonshine-v2", Object: "model", OwnedBy: "openai"},
		{ID: "whisper-1", Object: "model", OwnedBy: "openai"},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	models, err := stt.New(srv.URL).Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if models.Object != "list" || len(models.Data) != 2 {
		t.Errorf("unexpected models: %+v", models)
	}
	if models.Data[0].ID != "moonshine-v2" {
		t.Errorf("data[0].id = %q, want moonshine-v2", models.Data[0].ID)
	}
}

func TestOptionsPassedAsFormFields(t *testing.T) {
	received := make(map[string]string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		for _, key := range []string{"punctuate", "smart_format", "diarize", "keywords", "custom_spelling"} {
			if v := r.FormValue(key); v != "" {
				received[key] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stt.Response{Text: "ok"})
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("fake audio"), 0o644)

	client := stt.New(srv.URL,
		stt.WithPunctuate(true),
		stt.WithSmartFormat(false),
		stt.WithDiarize(true),
		stt.WithKeywords([]string{"hello", "world"}),
		stt.WithCustomSpelling(map[string]string{"colour": "color"}),
	)
	if _, err := client.Transcribe(context.Background(), tmp); err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	if got := received["punctuate"]; got != "true" {
		t.Errorf("punctuate = %q, want true", got)
	}
	if got := received["smart_format"]; got != "false" {
		t.Errorf("smart_format = %q, want false", got)
	}
	if got := received["diarize"]; got != "true" {
		t.Errorf("diarize = %q, want true", got)
	}
	var kw []string
	if err := json.Unmarshal([]byte(received["keywords"]), &kw); err != nil {
		t.Fatalf("unmarshal keywords: %v", err)
	}
	if len(kw) != 2 || kw[0] != "hello" || kw[1] != "world" {
		t.Errorf("keywords = %v, want [hello world]", kw)
	}
	var cs map[string]string
	if err := json.Unmarshal([]byte(received["custom_spelling"]), &cs); err != nil {
		t.Fatalf("unmarshal custom_spelling: %v", err)
	}
	if cs["colour"] != "color" {
		t.Errorf("custom_spelling[colour] = %q, want color", cs["colour"])
	}
}
