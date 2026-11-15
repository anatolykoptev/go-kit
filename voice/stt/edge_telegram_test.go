package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestTranscribeURLContextCancel(t *testing.T) {
	t.Parallel()
	audioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("audio data"))
	}))
	defer audioSrv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before any request

	_, err := stt.New("http://127.0.0.1:1").TranscribeURL(ctx, audioSrv.URL+"/voice.ogg")
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestTranscribeURLServerTimeout(t *testing.T) {
	t.Parallel()
	audioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("audio"))
	}))
	defer audioSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := stt.New("http://127.0.0.1:1").TranscribeURL(ctx, audioSrv.URL+"/voice.ogg")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestTranscribeURLEmptyBody(t *testing.T) {
	t.Parallel()
	audioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/ogg")
		// return empty body — valid 200 but no audio data
	}))
	defer audioSrv.Close()

	sttSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"","duration":0}`))
	}))
	defer sttSrv.Close()

	client := stt.New(sttSrv.URL)
	resp, err := client.TranscribeURL(context.Background(), audioSrv.URL+"/voice.ogg")
	if err != nil {
		t.Fatalf("unexpected error for empty audio body: %v", err)
	}
	if resp.Text != "" {
		t.Errorf("text = %q, want empty string", resp.Text)
	}
}
