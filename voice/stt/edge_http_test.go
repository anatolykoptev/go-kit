package stt_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("read error") }

func TestTranscribeInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()
	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)
	if _, err := stt.New(srv.URL).Transcribe(context.Background(), tmp); err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestTranscribeEmptyBody200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	_, err := stt.New(srv.URL).Transcribe(context.Background(), tmp)
	if err == nil {
		t.Fatal("expected unmarshal error for empty body")
	}
}

func TestTranscribeContextTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := stt.New(srv.URL).Transcribe(ctx, tmp)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestTranscribeEmptyFile(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"","duration":0}`))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "empty.wav")
	_ = os.WriteFile(tmp, []byte{}, 0o644)

	_, err := stt.New(srv.URL).Transcribe(context.Background(), tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected server to be called even with empty file")
	}
}

func TestTranscribeVerboseInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	_, err := stt.New(srv.URL).TranscribeVerbose(context.Background(), tmp)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestTranscribeReaderFails(t *testing.T) {
	t.Parallel()
	// errReader fails during multipart body build (io.Copy inside buildMultipart).
	_, err := stt.New("http://127.0.0.1:1").TranscribeReader(context.Background(), errReader{}, "bad.wav")
	if err == nil {
		t.Fatal("expected copy error")
	}
}

func TestModelsInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := stt.New(srv.URL).Models(context.Background())
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestModelsHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service down"))
	}))
	defer srv.Close()

	_, err := stt.New(srv.URL).Models(context.Background())
	var sttErr *stt.Error
	if !errors.As(err, &sttErr) {
		t.Fatalf("expected *stt.Error, got %T: %v", err, err)
	}
	if sttErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want 503", sttErr.StatusCode)
	}
}

func TestTranscribeRawHTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	_, err := stt.New(srv.URL).TranscribeRaw(context.Background(), tmp)
	var sttErr *stt.Error
	if !errors.As(err, &sttErr) {
		t.Fatalf("expected *stt.Error, got %T: %v", err, err)
	}
	if sttErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", sttErr.StatusCode)
	}
}

func TestTranscribeServerDropsConnection(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("hijack not supported")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "audio.wav")
	_ = os.WriteFile(tmp, []byte("audio"), 0o644)

	_, err := stt.New(srv.URL).Transcribe(context.Background(), tmp)
	if err == nil {
		t.Fatal("expected error when server drops connection")
	}
}
