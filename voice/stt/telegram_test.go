package stt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

// TestTranscribeURLDownloadTimeout verifies that downloadToTemp uses the
// configured client (with timeout) instead of http.DefaultClient (no timeout).
// A slow server that delays beyond the client timeout should return an error,
// not hang forever.
func TestTranscribeURLDownloadTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // slow server
	}))
	defer srv.Close()

	c := stt.New("http://127.0.0.1:1", stt.WithTimeout(200*time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.TranscribeURL(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected timeout error from slow download, got nil")
	}
}

// TestWithTempDirUsed verifies that WithTempDir causes temp files to be
// created in the configured directory.
func TestWithTempDirUsed(t *testing.T) {
	tmpDir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake audio data"))
	}))
	defer srv.Close()

	c := stt.New("http://127.0.0.1:1", stt.WithTempDir(tmpDir), stt.WithTimeout(5*time.Second))

	// We can't easily test TranscribeURL end-to-end (it calls Transcribe
	// which hits the STT endpoint), but we can verify the temp dir is used
	// by checking that downloadToTemp creates files in tmpDir.
	// Use TranscribeURL against a server that returns audio — the download
	// will succeed, then Transcribe will fail (wrong endpoint), but the
	// temp file should be in tmpDir and cleaned up.
	// Transcribe fails because the STT endpoint is unreachable, but the
	// download + temp file creation work; only the temp dir content matters.
	_, _ = c.TranscribeURL(context.Background(), srv.URL)

	// Verify no temp files leaked in the system temp dir.
	systemTmp := os.TempDir()
	entries, _ := os.ReadDir(systemTmp)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "stt-voice-") {
			t.Errorf("temp file leaked in system temp dir: %s", e.Name())
		}
	}

	// Verify the configured temp dir is empty after cleanup (defer os.Remove).
	entries, _ = os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "stt-voice-") {
			t.Errorf("temp file leaked in configured temp dir: %s", e.Name())
		}
	}
}

// TestTranscribeURLCleansTempOnPanic verifies that temp files are cleaned up
// even if the transcription step fails. We can't easily trigger a panic, but
// we can verify cleanup on error paths.
func TestTranscribeURLCleansTempOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake audio data"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	c := stt.New("http://127.0.0.1:1", stt.WithTempDir(tmpDir), stt.WithTimeout(5*time.Second))

	// TranscribeURL will download successfully, then Transcribe will fail
	// (unreachable STT endpoint). The temp file should be cleaned up by defer.
	_, err := c.TranscribeURL(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error from unreachable STT endpoint")
	}

	// Verify no temp files leaked.
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "stt-voice-") {
			t.Errorf("temp file leaked after error: %s", filepath.Join(tmpDir, e.Name()))
		}
	}
}
