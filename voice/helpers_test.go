package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	testKey    = "test-key-7f3a91"
	secretText = "секретная фраза номер 4471"
)

// syncBuf is a goroutine-safe log sink.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLog returns a logger writing every level to the returned buffer.
func captureLog() (*slog.Logger, *syncBuf) {
	buf := &syncBuf{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// wavBytes builds a canonical 44-byte-header mono 16-bit WAV.
func wavBytes(pcm []byte, rate int) []byte {
	le := binary.LittleEndian
	b := []byte("RIFF")
	b = le.AppendUint32(b, uint32(36+len(pcm)))
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1) // PCM
	b = le.AppendUint16(b, 1) // mono
	b = le.AppendUint32(b, uint32(rate))
	b = le.AppendUint32(b, uint32(rate*2))
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(len(pcm)))
	return append(b, pcm...)
}

// reply writes a canned response body.
func reply(w http.ResponseWriter, body string) { _, _ = w.Write([]byte(body)) }

func geminiJSON(audio []byte) string {
	return `{"steps":[{"type":"user_input","content":[]},{"type":"model_output","content":[{"type":"audio","data":"` +
		base64.StdEncoding.EncodeToString(audio) + `"}]}]}`
}

type fakeEncoded struct {
	pcm  []byte
	rate int
}

// encoderLog records encoder calls; safe for use from the provider goroutine.
type encoderLog struct {
	mu    sync.Mutex
	calls []fakeEncoded
}

func (e *encoderLog) add(f fakeEncoded) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, f)
}

func (e *encoderLog) len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

func (e *encoderLog) get(i int) fakeEncoded {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls[i]
}

// newTestGemini returns a provider pointed at srv whose encoder records PCM
// instead of running ffmpeg and whose output goes to a per-test temp dir.
func newTestGemini(t testing.TB, srv *httptest.Server, key string, enc *encoderLog) *GeminiProvider {
	t.Helper()
	p := NewGeminiProvider(GeminiConfig{
		APIKey: key, baseURL: srv.URL, HTTPClient: srv.Client(), OutputDir: t.TempDir(),
	})
	p.ffmpegOK = func() bool { return true }
	p.encode = func(_ context.Context, pcm []byte, rate int, out string) error {
		enc.add(fakeEncoded{append([]byte(nil), pcm...), rate})
		return os.WriteFile(out, []byte("ogg"), 0o644)
	}
	return p
}

type stubProvider struct {
	calls atomic.Int32
	avail bool
	path  string
	err   error
}

func (s *stubProvider) Synthesize(context.Context, string, string) (string, error) {
	s.calls.Add(1)
	return s.path, s.err
}

func (s *stubProvider) IsAvailable() bool { return s.avail }
