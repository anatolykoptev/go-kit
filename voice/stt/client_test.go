package stt_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestTranscribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("parse multipart: %v", err)
		}

		if got := r.FormValue("model"); got != "moonshine-v2" {
			t.Errorf("model = %q, want moonshine-v2", got)
		}
		if got := r.FormValue("language"); got != "ru" {
			t.Errorf("language = %q, want ru", got)
		}
		if got := r.FormValue("response_format"); got != "json" {
			t.Errorf("response_format = %q, want json", got)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		if header.Filename != "test.wav" {
			t.Errorf("filename = %q, want test.wav", header.Filename)
		}
		data, _ := io.ReadAll(file)
		if string(data) != "fake audio" {
			t.Errorf("file content = %q", string(data))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stt.Response{
			Text:     "привет мир",
			Duration: 1.5,
		})
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "test.wav")
	_ = os.WriteFile(tmp, []byte("fake audio"), 0o644)

	client := stt.New(srv.URL)
	resp, err := client.Transcribe(context.Background(), tmp)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if resp.Text != "привет мир" {
		t.Errorf("text = %q, want привет мир", resp.Text)
	}
	if resp.Duration != 1.5 {
		t.Errorf("duration = %v, want 1.5", resp.Duration)
	}
	if resp.Language != "ru" {
		t.Errorf("language = %q, want ru", resp.Language)
	}
}

func TestTranscribeWithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(10 << 20)
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Errorf("model = %q, want whisper-1", got)
		}
		if got := r.FormValue("language"); got != "en" {
			t.Errorf("language = %q, want en", got)
		}
		if got := r.FormValue("response_format"); got != "verbose_json" {
			t.Errorf("response_format = %q, want verbose_json", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stt.Response{Text: "hello", Language: "en", Duration: 2.0})
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "test.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	client := stt.New(srv.URL,
		stt.WithLanguage("en"),
		stt.WithModel("whisper-1"),
		stt.WithFormat("verbose_json"),
	)
	resp, err := client.Transcribe(context.Background(), tmp)
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if resp.Text != "hello" {
		t.Errorf("text = %q", resp.Text)
	}
	if resp.Language != "en" {
		t.Errorf("language = %q", resp.Language)
	}
}

func TestTranscribeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("model not loaded"))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "test.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	client := stt.New(srv.URL)
	_, err := client.Transcribe(context.Background(), tmp)
	if err == nil {
		t.Fatal("expected error")
	}
	var sttErr *stt.Error
	if !errors.As(err, &sttErr) {
		t.Fatalf("expected *stt.Error, got %T: %v", err, err)
	}
	if sttErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", sttErr.StatusCode, http.StatusInternalServerError)
	}
	if sttErr.Message != "Internal Server Error" {
		t.Errorf("Message = %q, want the status text only", sttErr.Message)
	}
	if strings.Contains(err.Error(), "model not loaded") {
		t.Errorf("server body leaked into error: %v", err)
	}
}

func TestTranscribeFileNotFound(t *testing.T) {
	client := stt.New("http://localhost:1")
	_, err := client.Transcribe(context.Background(), "/nonexistent/file.wav")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := stt.New(srv.URL)
	if !client.IsAvailable() {
		t.Error("expected available")
	}
}

func TestIsAvailableDown(t *testing.T) {
	client := stt.New("http://127.0.0.1:1", stt.WithTimeout(100*time.Millisecond))
	if client.IsAvailable() {
		t.Error("expected not available")
	}
}
