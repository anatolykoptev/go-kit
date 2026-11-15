package voice

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGemini_RequestShape(t *testing.T) {
	var gotReq *http.Request
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		reply(w, geminiJSON(wavBytes([]byte{1, 0, 2, 0}, 24000)))
	}))
	defer srv.Close()
	enc := &encoderLog{}
	p := newTestGemini(t, srv, testKey, enc)
	p.cfg.Voice, p.cfg.Style, p.cfg.Model = "Zephyr", "bright", "gemini-test-tts"

	path, err := p.Synthesize(context.Background(), "Привет, мир.", "ru")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	if gotReq.Method != http.MethodPost || gotReq.URL.Path != "/v1beta/interactions" {
		t.Errorf("got %s %s", gotReq.Method, gotReq.URL.Path)
	}
	if gotReq.Header.Get("x-goog-api-key") != testKey {
		t.Errorf("api key header = %q", gotReq.Header.Get("x-goog-api-key"))
	}
	if gotReq.URL.RawQuery != "" || strings.Contains(gotReq.URL.String(), testKey) {
		t.Errorf("key or query leaked into URL: %s", gotReq.URL)
	}
	if gotBody["model"] != "gemini-test-tts" {
		t.Errorf("model = %v", gotBody["model"])
	}
	rf, _ := gotBody["response_format"].(map[string]any)
	if rf["type"] != "audio" || rf["mime_type"] != "audio/wav" {
		t.Errorf("response_format = %v", gotBody["response_format"])
	}
	sc := gotBody["generation_config"].(map[string]any)["speech_config"].([]any)[0].(map[string]any)
	if sc["voice"] != "Zephyr" {
		t.Errorf("voice = %v", sc["voice"])
	}
	in := gotBody["input"].([]any)[0].(map[string]any)
	part := in["content"].([]any)[0].(map[string]any)
	ann := part["annotations"].([]any)[0].(map[string]any)
	if in["type"] != "user_input" || part["text"] != "Привет, мир." ||
		ann["type"] != "speech_metadata" || ann["style"] != "bright" {
		t.Errorf("input shape wrong: %v", in)
	}
}

func TestGemini_AudioFormats(t *testing.T) {
	pcm := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	cases := []struct {
		name     string
		audio    []byte
		wantPCM  []byte
		wantRate int
	}{
		{"wav default", wavBytes(pcm, 24000), pcm, 24000},
		{"wav other rate", wavBytes(pcm, 16000), pcm, 16000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reply(w, geminiJSON(c.audio))
			}))
			defer srv.Close()
			enc := &encoderLog{}
			path, err := newTestGemini(t, srv, testKey, enc).Synthesize(context.Background(), "hi", "")
			if err != nil {
				t.Fatal(err)
			}
			os.Remove(path)
			if enc.len() != 1 || !bytes.Equal(enc.get(0).pcm, c.wantPCM) || enc.get(0).rate != c.wantRate {
				t.Errorf("encoder got %+v, want pcm=%v rate=%d", enc.calls, c.wantPCM, c.wantRate)
			}
		})
	}
}

func TestGemini_RejectsUnsupportedWAVAndEmptyAudio(t *testing.T) {
	stereo := wavBytes([]byte{0, 0, 0, 0}, 24000)
	binary.LittleEndian.PutUint16(stereo[22:], 2) // channels
	for name, body := range map[string]string{
		"stereo wav":  geminiJSON(stereo),
		"raw pcm":     geminiJSON([]byte{1, 0, 2, 0, 3, 0}),
		"no audio":    `{"steps":[{"type":"model_output","content":[{"type":"text","data":"x"}]}]}`,
		"bad json":    `not json`,
		"bad base64":  `{"steps":[{"type":"model_output","content":[{"type":"audio","data":"!!!"}]}]}`,
		"empty steps": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reply(w, body) }))
			defer srv.Close()
			enc := &encoderLog{}
			if path, err := newTestGemini(t, srv, testKey, enc).Synthesize(context.Background(), "hi", ""); err == nil {
				os.Remove(path)
				t.Fatal("want error")
			}
			if enc.len() != 0 {
				t.Error("encoder must not run on a bad response")
			}
		})
	}
}

func TestGemini_RealFFmpegProducesOggOpus(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("ffmpeg missing under CI: provision it, do not skip")
		}
		t.Skip("ffmpeg not installed on this host")
	}
	pcm := make([]byte, 24000*2/2) // 0.5 s of silence
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(w, geminiJSON(wavBytes(pcm, 24000)))
	}))
	defer srv.Close()
	p := NewGeminiProvider(GeminiConfig{APIKey: testKey, baseURL: srv.URL, HTTPClient: srv.Client(), OutputDir: t.TempDir()})
	path, err := p.Synthesize(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.HasPrefix(data, []byte("OggS")) || !bytes.Contains(data[:min(len(data), 256)], []byte("OpusHead")) {
		t.Errorf("not ogg/opus: % x", data[:min(len(data), 32)])
	}
}

func TestGemini_ErrorReasonLoggedWithoutMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		reply(w, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"`+secretText+`"}}`)
	}))
	defer srv.Close()
	enc := &encoderLog{}
	edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
	log, buf := captureLog()

	if _, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), edge, log).
		Synthesize(context.Background(), secretText, "ru"); err != nil {
		t.Fatal(err)
	}
	dump := buf.String()
	if !strings.Contains(dump, "INVALID_ARGUMENT") || !strings.Contains(dump, "400") {
		t.Errorf("status/reason not logged: %s", dump)
	}
	if strings.Contains(dump, secretText) {
		t.Errorf("error message leaked into log: %s", dump)
	}
}

func TestGoogleErrorReason(t *testing.T) {
	for in, want := range map[string]string{
		`{"error":{"status":"RESOURCE_EXHAUSTED"}}`:                      "RESOURCE_EXHAUSTED",
		`{"error":{"message":"secret text","code":"too_many_requests"}}`: "too_many_requests",
		`{"error":{"message":"m","code":429}}`:                           "",
		`{"error":{"code":"Free text, with spaces"}}`:                    "",
		`{"error":{"code":"UPPER"}}`:                                     "",
		`{"error":{"status":"free text with spaces"}}`:                   "",
		`{"error":{"status":"lower_case"}}`:                              "",
		`not json`:                                                       "",
		`{}`:                                                             "",
	} {
		if got := googleErrorReason([]byte(in)); got != want {
			t.Errorf("%s -> %q, want %q", in, got, want)
		}
	}
}

func TestGemini_DoesNotFollowRedirectsWithKey(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	enc := &encoderLog{}
	_, err := newTestGemini(t, srv, testKey, enc).Synthesize(context.Background(), "hi", "")
	var he *GeminiHTTPError
	if !errors.As(err, &he) || he.Status != http.StatusTemporaryRedirect || leaked.Load() != 0 {
		t.Errorf("err=%v redirected-requests=%d", err, leaked.Load())
	}
}

func TestGemini_TextCap(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqs.Add(1)
		reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
	}))
	defer srv.Close()
	enc := &encoderLog{}
	p := newTestGemini(t, srv, testKey, enc)

	// Cyrillic runes are 2 bytes each: the cap must count runes, not bytes.
	path, err := p.Synthesize(context.Background(), strings.Repeat("а", geminiMaxTextRunes), "")
	if err != nil || reqs.Load() != 1 {
		t.Fatalf("at the cap: err=%v requests=%d, want one request", err, reqs.Load())
	}
	os.Remove(path)
	reqs.Store(0)
	_, err = p.Synthesize(context.Background(), strings.Repeat("а", geminiMaxTextRunes+1), "")
	if !errors.Is(err, ErrTextTooLong) || reqs.Load() != 0 {
		t.Errorf("over the cap: err=%v requests=%d, want ErrTextTooLong and no request", err, reqs.Load())
	}
	if _, err := p.Synthesize(context.Background(), " \n ", ""); err == nil || reqs.Load() != 0 {
		t.Errorf("blank text: err=%v requests=%d", err, reqs.Load())
	}
}

func TestGemini_BareProviderUsesFullParentBudget(t *testing.T) {
	// A bare provider (fallback "none") must not give up the edge reserve: a
	// parent deadline just above the reserve still gets a request.
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqs.Add(1)
		reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
	}))
	defer srv.Close()
	enc := &encoderLog{}
	ctx, cancel := context.WithTimeout(context.Background(), EdgeReserve+500*time.Millisecond)
	defer cancel()
	path, err := newTestGemini(t, srv, testKey, enc).Synthesize(ctx, "hi", "")
	if err != nil || reqs.Load() != 1 {
		t.Fatalf("err=%v requests=%d", err, reqs.Load())
	}
	os.Remove(path)
}

func TestGemini_OutputDirHonoredAndUnique(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
	}))
	defer srv.Close()
	enc := &encoderLog{}
	p := newTestGemini(t, srv, testKey, enc)
	p.cfg.OutputDir = t.TempDir() + "/nested/out" // created on demand

	a, err := p.Synthesize(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Synthesize(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two syntheses returned the same path %s", a)
	}
	for _, f := range []string{a, b} {
		if !strings.HasPrefix(f, p.cfg.OutputDir+"/") {
			t.Errorf("%s not under OutputDir %s", f, p.cfg.OutputDir)
		}
	}
}

// A late success must not spend the budget a fallback needs: when the request
// returns with under 3 s left, the encode is skipped.
func TestGemini_LateSuccessSkipsEncode(t *testing.T) {
	const delay = 1500 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
	}))
	defer srv.Close()

	t.Run("bare provider keeps the paid synthesis", func(t *testing.T) {
		enc := &encoderLog{}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second) // 2.5 s left after the reply
		defer cancel()
		path, err := newTestGemini(t, srv, testKey, enc).Synthesize(ctx, "hi", "")
		if err != nil || enc.len() != 1 {
			t.Fatalf("err=%v encodes=%d, want a successful encode", err, enc.len())
		}
		os.Remove(path)
	})
	t.Run("positive control: enough budget encodes", func(t *testing.T) {
		enc := &encoderLog{}
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second) // 5.5 s left after the reply
		defer cancel()
		path, err := newTestGemini(t, srv, testKey, enc).Synthesize(ctx, "hi", "")
		if err != nil || enc.len() != 1 {
			t.Fatalf("err=%v encodes=%d, want one encode", err, enc.len())
		}
		os.Remove(path)
	})
	t.Run("through fallback edge still runs", func(t *testing.T) {
		enc := &encoderLog{}
		edge := &stubProvider{avail: true, path: "/tmp/edge.ogg"}
		ctx, cancel := context.WithTimeout(context.Background(), EdgeReserve+4*time.Second)
		defer cancel()
		got, err := NewFallbackProvider("gemini", newTestGemini(t, srv, testKey, enc), edge, nil).Synthesize(ctx, "hi", "")
		if err != nil || got != "/tmp/edge.ogg" || edge.calls.Load() != 1 || enc.len() != 0 {
			t.Errorf("got %q err=%v edgeCalls=%d encodes=%d", got, err, edge.calls.Load(), enc.len())
		}
	})
}

// A well-formed WAV body behind a wrong magic must be rejected, not parsed.
func TestGemini_NonRIFFRejectedEvenIfOtherwiseValid(t *testing.T) {
	bad := wavBytes([]byte{1, 0, 2, 0}, 24000)
	copy(bad[0:4], "RIFX")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reply(w, geminiJSON(bad)) }))
	defer srv.Close()
	enc := &encoderLog{}
	if path, err := newTestGemini(t, srv, testKey, enc).Synthesize(context.Background(), "hi", ""); err == nil || enc.len() != 0 {
		os.Remove(path)
		t.Errorf("err=%v encodes=%d, want rejection without encoding", err, enc.len())
	}
}

// Streamed WAVs may carry 0xFFFFFFFF as the data size: read to the end.
func TestDecodeGeminiAudio_StreamedSizes(t *testing.T) {
	w := wavBytes([]byte{1, 0, 2, 0, 3}, 24000) // odd trailing byte is dropped
	binary.LittleEndian.PutUint32(w[40:], 0xFFFFFFFF)
	pcm, rate, err := decodeGeminiAudio(w)
	if err != nil || rate != 24000 || !bytes.Equal(pcm, []byte{1, 0, 2, 0}) {
		t.Errorf("pcm=%v rate=%d err=%v", pcm, rate, err)
	}
}

// The provider's own logs (success and failure) never carry text or key.
func TestGemini_OwnLogsNeverContainTextOrKey(t *testing.T) {
	for name, status := range map[string]int{"success": http.StatusOK, "failure": http.StatusBadRequest} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				if status == http.StatusOK {
					reply(w, geminiJSON(wavBytes([]byte{1, 0}, 24000)))
					return
				}
				reply(w, `{"error":{"status":"INVALID_ARGUMENT","message":"`+secretText+`"}}`)
			}))
			defer srv.Close()
			enc := &encoderLog{}
			p := newTestGemini(t, srv, testKey, enc)
			log, buf := captureLog()
			p.log = log
			if path, err := p.Synthesize(context.Background(), secretText, "ru"); err == nil {
				os.Remove(path)
			}
			dump := buf.String()
			if !strings.Contains(dump, "text_runes=") {
				t.Fatalf("provider logged nothing useful: %q", dump)
			}
			if strings.Contains(dump, secretText) || strings.Contains(dump, testKey) {
				t.Errorf("log leaks text or key: %s", dump)
			}
		})
	}
}
