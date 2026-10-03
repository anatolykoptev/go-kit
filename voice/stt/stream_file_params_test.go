package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/goleak"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestStreamFilePassesAllParams verifies that StreamFile passes VAD,
// SmartFormat, Punctuate, and Encoding through to the WebSocket URL.
func TestStreamFilePassesAllParams(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		conn, err := (&websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}).Upgrade(w, r, nil) //nolint:mnd
		if err != nil {
			return
		}
		defer conn.Close()
		// Drain until close.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	// Write a tiny PCM file.
	f, err := os.CreateTemp(t.TempDir(), "audio*.pcm")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.Write(make([]byte, 3200)) // 100ms of 16kHz s16le
	_ = f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, _ = stt.StreamFile(
		ctx,
		"ws://"+strings.TrimPrefix(srv.URL, "http://"),
		f.Name(),
		stt.WithStreamLanguage("en"),
		stt.WithChunkDuration(50*time.Millisecond),
		stt.WithStreamSampleRate(16000),
		stt.WithInterim(false),
		stt.WithStreamVAD(false),
		stt.WithStreamSmartFormat(true),
		stt.WithStreamPunctuate(false),
		stt.WithStreamEncoding("pcm_f32le"),
	)

	// Parse the query string from the URL.
	if !strings.Contains(gotURL, "vad=false") {
		t.Errorf("URL %q missing vad=false", gotURL)
	}
	if !strings.Contains(gotURL, "smart_format=true") {
		t.Errorf("URL %q missing smart_format=true", gotURL)
	}
	if !strings.Contains(gotURL, "punctuate=false") {
		t.Errorf("URL %q missing punctuate=false", gotURL)
	}
	if !strings.Contains(gotURL, "encoding=pcm_f32le") {
		t.Errorf("URL %q missing encoding=pcm_f32le", gotURL)
	}

	// Verify goleak doesn't catch client-side leaks.
	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })
}

// TestStreamFileParamsDefaultVAD verifies that VAD defaults to true (nil)
// when WithStreamVAD is not used.
func TestStreamFileParamsDefaultVAD(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		conn, err := (&websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}).Upgrade(w, r, nil) //nolint:mnd
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	f, err := os.CreateTemp(t.TempDir(), "audio*.pcm")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	_, _ = f.Write(make([]byte, 3200))
	_ = f.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, _ = stt.StreamFile(ctx, "ws://"+strings.TrimPrefix(srv.URL, "http://"), f.Name())

	// VAD should default to true.
	if !strings.Contains(gotURL, "vad=true") {
		t.Errorf("URL %q missing vad=true (default)", gotURL)
	}
	// Punctuate should default to true.
	if !strings.Contains(gotURL, "punctuate=true") {
		t.Errorf("URL %q missing punctuate=true (default)", gotURL)
	}

	opts := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, opts) })
}
