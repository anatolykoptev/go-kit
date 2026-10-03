package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultGeminiModel is the TTS model used when GeminiConfig.Model is empty.
	DefaultGeminiModel = "gemini-3.8-flash-tts"
	// DefaultGeminiVoice is the prebuilt voice used when GeminiConfig.Voice is empty.
	DefaultGeminiVoice = "Aoede"
	// DefaultGeminiStyle is the delivery style used when GeminiConfig.Style is empty.
	DefaultGeminiStyle = "warm, calm and friendly, at a natural conversational pace"

	geminiBaseURL = "https://generativelanguage.googleapis.com"

	// geminiMaxTextRunes caps the input of the single request a reply makes.
	// The docs state no limit; a live 2000-rune request took ~35 s for ~136 s of
	// audio, so longer texts would not fit a voice-reply deadline. Above the cap
	// ErrTextTooLong lets a caller (or [FallbackProvider]) pick another provider.
	geminiMaxTextRunes = 2000

	// minGeminiBudget is the least time left on the context deadline worth
	// starting a Gemini request for, and worth encoding its answer for.
	minGeminiBudget = 3 * time.Second

	geminiRequestTimeout = 45 * time.Second
	geminiMaxResponse    = 32 << 20 // bytes; 24 kHz mono 16-bit is ~48 KB/s
	geminiMaxErrorBody   = 64 << 10 // bytes read from a non-2xx body to find error.status
)

// ErrTextTooLong is returned by [GeminiProvider] for text above its cap of
// 2000 runes.
var ErrTextTooLong = errors.New("gemini tts: text exceeds 2000-rune cap")

// ErrBudgetExhausted means the context deadline leaves too little time (under
// 3 s) to start a Gemini request or to encode its answer.
var ErrBudgetExhausted = errors.New("gemini tts: deadline budget exhausted")

// GeminiHTTPError is a non-2xx answer from the Gemini API. It carries the
// status only: response bodies are never surfaced, so nothing user-derived
// can reach logs through it.
type GeminiHTTPError struct {
	Status int
	// Reason is Google's error.status enum (e.g. INVALID_ARGUMENT) or the
	// Interactions API's error.code token (e.g. too_many_requests), kept only if
	// it matches a strict token pattern; never the message or body.
	Reason string
}

func (e *GeminiHTTPError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("gemini tts: http status %d (%s)", e.Status, e.Reason)
	}
	return fmt.Sprintf("gemini tts: http status %d", e.Status)
}

var (
	googleStatusEnum = regexp.MustCompile(`^[A-Z][A-Z_]{2,39}$`)
	googleCodeToken  = regexp.MustCompile(`^[a-z][a-z_]{2,39}$`)
)

// googleErrorReason extracts a machine-readable reason from an API error body,
// never the message. Two shapes are seen: the classic Google
// {"error":{"status":"INVALID_ARGUMENT"}} and the Interactions API's
// {"error":{"message":...,"code":"too_many_requests"}}. A value is kept only if
// it is a strict enum/token; anything else yields "".
func googleErrorReason(body []byte) string {
	var e struct {
		Error struct {
			Status string          `json:"status"`
			Code   json.RawMessage `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	if googleStatusEnum.MatchString(e.Error.Status) {
		return e.Error.Status
	}
	var code string // a numeric code (classic shape) fails to unmarshal and is ignored
	if json.Unmarshal(e.Error.Code, &code) == nil && googleCodeToken.MatchString(code) {
		return code
	}
	return ""
}

// GeminiConfig configures [GeminiProvider].
type GeminiConfig struct {
	// APIKey is the Gemini Developer API key. Required: the provider is
	// unavailable without it. The caller supplies it; it is sent only in the
	// x-goog-api-key header.
	APIKey string
	Model  string // default DefaultGeminiModel
	Voice  string // prebuilt voice name, default DefaultGeminiVoice
	Style  string // turn-level delivery style, default DefaultGeminiStyle
	// OutputDir is where audio files are created (made if missing). Empty
	// means the OS temp dir.
	OutputDir string
	// Logger receives operational logs; nil means slog.Default().
	Logger *slog.Logger
	// baseURL overrides the API origin (tests only).
	baseURL string
	// HTTPClient supplies the transport (proxy, TLS, timeouts). Its
	// CheckRedirect is always replaced: redirects are never followed.
	HTTPClient *http.Client
}

// GeminiProvider synthesizes speech through the Gemini Developer API
// (POST /v1beta/interactions) and encodes it to ogg/opus.
type GeminiProvider struct {
	cfg    GeminiConfig
	client *http.Client
	log    *slog.Logger
	// encode turns 16-bit mono PCM into an ogg/opus file; a field so tests can
	// observe the PCM without ffmpeg.
	encode func(ctx context.Context, pcm []byte, rate int, outPath string) error
	// ffmpegOK reports whether ffmpeg is on PATH.
	ffmpegOK func() bool
}

// NewGeminiProvider returns a GeminiProvider.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	if cfg.Model == "" {
		cfg.Model = DefaultGeminiModel
	}
	if cfg.Voice == "" {
		cfg.Voice = DefaultGeminiVoice
	}
	if cfg.Style == "" {
		cfg.Style = DefaultGeminiStyle
	}
	if cfg.baseURL == "" {
		cfg.baseURL = geminiBaseURL
	}
	var c http.Client
	if cfg.HTTPClient != nil {
		c = *cfg.HTTPClient
	}
	// Never follow redirects: x-goog-api-key must not be forwarded anywhere.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	log := loggerOrDefault(cfg.Logger)
	return &GeminiProvider{
		cfg:    cfg,
		client: &c,
		log:    log,
		encode: func(ctx context.Context, pcm []byte, rate int, out string) error {
			return encodeOggOpus(ctx, log, pcm, rate, out)
		},
		ffmpegOK: func() bool {
			_, err := exec.LookPath("ffmpeg")
			return err == nil
		},
	}
}

// IsAvailable is true only when a key is configured and ffmpeg can encode the
// result; it does not probe the network.
func (p *GeminiProvider) IsAvailable() bool {
	return p.cfg.APIKey != "" && p.ffmpegOK()
}

// Synthesize implements [Provider]; lang is ignored (the model detects it).
// It returns [ErrTextTooLong] above 2000 runes and [ErrBudgetExhausted] when
// the ctx deadline leaves under 3 s before the request or after it.
func (p *GeminiProvider) Synthesize(ctx context.Context, text, _ string) (string, error) {
	if p.cfg.APIKey == "" {
		return "", errors.New("gemini tts: api key not configured")
	}
	if utf8.RuneCountInString(text) > geminiMaxTextRunes {
		return "", ErrTextTooLong
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("gemini tts: empty text")
	}
	if budgetLow(ctx) {
		return "", ErrBudgetExhausted
	}
	p.log.InfoContext(ctx, "voice: synthesizing", "provider", "gemini",
		"model", p.cfg.Model, "voice", p.cfg.Voice, "text_runes", utf8.RuneCountInString(text))

	audio, err := p.request(ctx, text)
	if err != nil {
		return "", err
	}
	pcm, rate, err := decodeGeminiAudio(audio)
	if err != nil {
		return "", err
	}
	// Under a FallbackProvider a late success must not eat the time the
	// fallback needs: skip the encode when too little of the deadline remains.
	// A bare provider keeps a paid, successful synthesis.
	if underFallback(ctx) && budgetLow(ctx) {
		return "", ErrBudgetExhausted
	}
	outPath, err := createTempFile(p.cfg.OutputDir, "tts_*.ogg", "gemini tts")
	if err != nil {
		return "", err
	}
	if err := p.encode(ctx, pcm, rate, outPath); err != nil {
		os.Remove(outPath)
		return "", err
	}
	p.log.InfoContext(ctx, "voice: speech synthesized", "provider", "gemini")
	return outPath, nil
}

type fallbackKey struct{}

// underFallback reports whether ctx was derived by FallbackProvider for its
// primary.
func underFallback(ctx context.Context) bool {
	v, _ := ctx.Value(fallbackKey{}).(bool)
	return v
}

// budgetLow reports whether ctx has a deadline less than minGeminiBudget away.
func budgetLow(ctx context.Context) bool {
	dl, ok := ctx.Deadline()
	return ok && time.Until(dl) < minGeminiBudget
}

type geminiAnnotation struct {
	Type  string `json:"type"`
	Style string `json:"style"`
}

type geminiTextPart struct {
	Type        string             `json:"type"`
	Text        string             `json:"text"`
	Annotations []geminiAnnotation `json:"annotations,omitempty"`
}

type geminiInput struct {
	Type    string           `json:"type"`
	Content []geminiTextPart `json:"content"`
}

type geminiSpeech struct {
	Voice string `json:"voice"`
}

type geminiRequest struct {
	Model          string            `json:"model"`
	Input          []geminiInput     `json:"input"`
	ResponseFormat map[string]string `json:"response_format"`
	GenerationCfg  struct {
		SpeechConfig []geminiSpeech `json:"speech_config"`
	} `json:"generation_config"`
}

// geminiResponse covers the REST shape (steps[].content[] audio blocks) and
// the SDK convenience shape (output_audio), the latter as a defensive fallback.
type geminiResponse struct {
	Steps []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Data string `json:"data"`
		} `json:"content"`
	} `json:"steps"`
	OutputAudio *struct {
		Data string `json:"data"`
	} `json:"output_audio"`
}

// request makes the single synthesis call and returns the decoded WAV bytes;
// the request pins response_format.mime_type=audio/wav.
func (p *GeminiProvider) request(ctx context.Context, text string) ([]byte, error) {
	part := geminiTextPart{Type: "text", Text: text}
	if p.cfg.Style != "" {
		part.Annotations = []geminiAnnotation{{Type: "speech_metadata", Style: p.cfg.Style}}
	}
	body := geminiRequest{
		Model:          p.cfg.Model,
		Input:          []geminiInput{{Type: "user_input", Content: []geminiTextPart{part}}},
		ResponseFormat: map[string]string{"type": "audio", "mime_type": "audio/wav"},
	}
	body.GenerationCfg.SpeechConfig = []geminiSpeech{{Voice: p.cfg.Voice}}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini tts: marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, geminiRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(p.cfg.baseURL, "/")+"/v1beta/interactions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gemini tts: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		// The key travels in a header only, so the wrapped url.Error holds none.
		return nil, fmt.Errorf("gemini tts: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
		eb, _ := io.ReadAll(io.LimitReader(resp.Body, geminiMaxErrorBody))
		return nil, &GeminiHTTPError{Status: resp.StatusCode, Reason: googleErrorReason(eb)}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, geminiMaxResponse))
	if err != nil {
		return nil, fmt.Errorf("gemini tts: read response: %w", err)
	}
	return parseGeminiAudio(raw)
}

// parseGeminiAudio extracts and base64-decodes the audio block of a response.
func parseGeminiAudio(raw []byte) ([]byte, error) {
	var out geminiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errors.New("gemini tts: malformed response json")
	}
	b64 := ""
	for _, s := range out.Steps {
		if s.Type != "model_output" {
			continue
		}
		for _, c := range s.Content {
			if c.Type == "audio" && c.Data != "" {
				b64 = c.Data // keep the LAST audio block, as the docs' convenience property does
			}
		}
	}
	if b64 == "" && out.OutputAudio != nil {
		b64 = out.OutputAudio.Data
	}
	if b64 == "" {
		return nil, errors.New("gemini tts: response has no audio")
	}
	audio, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("gemini tts: audio is not valid base64")
	}
	return audio, nil
}

const (
	wavHeaderLen   = 12 // "RIFF" size "WAVE"
	wavChunkHeader = 8  // id + size
	wavFmtMinLen   = 16
	wavFmtPCM      = 1
	wavBits16      = 16
)

// decodeGeminiAudio returns 16-bit mono PCM and its sample rate from the
// RIFF/WAV container requested via response_format.mime_type=audio/wav.
func decodeGeminiAudio(b []byte) (pcm []byte, rate int, err error) {
	if len(b) < wavHeaderLen || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		// We request audio/wav; anything else is unexpected, not guessable PCM.
		return nil, 0, errors.New("gemini tts: response is not a RIFF/WAVE file")
	}
	for off := wavHeaderLen; off+wavChunkHeader <= len(b); {
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4 : off+wavChunkHeader]))
		body := off + wavChunkHeader
		switch id {
		case "fmt ":
			if size < wavFmtMinLen || body+wavFmtMinLen > len(b) {
				return nil, 0, errors.New("gemini tts: truncated wav fmt chunk")
			}
			if rate, err = parseWavFmt(b[body : body+wavFmtMinLen]); err != nil {
				return nil, 0, err
			}
		case "data":
			if rate == 0 {
				return nil, 0, errors.New("gemini tts: wav data before fmt")
			}
			data := wavData(b, body, size)
			if len(data) < 2 {
				return nil, 0, errors.New("gemini tts: wav has no samples")
			}
			return data[:len(data)&^1], rate, nil
		}
		off = body + size + size&1
	}
	return nil, 0, errors.New("gemini tts: wav has no data chunk")
}

// wavData slices a data chunk; a streamed WAV may carry 0/0xFFFFFFFF sizes, so
// a size running past the buffer means "to the end".
func wavData(b []byte, body, size int) []byte {
	end := body + size
	if end > len(b) || end < body {
		end = len(b)
	}
	return b[body:end]
}

// parseWavFmt validates a 16-byte fmt chunk body (16-bit mono PCM) and
// returns its sample rate.
func parseWavFmt(f []byte) (int, error) {
	format := binary.LittleEndian.Uint16(f[0:2])
	channels := binary.LittleEndian.Uint16(f[2:4])
	rate := int(binary.LittleEndian.Uint32(f[4:8]))
	bits := binary.LittleEndian.Uint16(f[14:16])
	if format != wavFmtPCM || channels != 1 || bits != wavBits16 || rate <= 0 {
		return 0, fmt.Errorf("gemini tts: unsupported wav (format=%d channels=%d bits=%d)", format, channels, bits)
	}
	return rate, nil
}

// encodeOggOpus pipes 16-bit mono PCM to ffmpeg (argv slice, no shell) and
// writes ogg/opus (what Telegram renders as a voice note).
func encodeOggOpus(ctx context.Context, log *slog.Logger, pcm []byte, rate int, outPath string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", //nolint:gosec // fixed binary, argv slice, no shell
		"-f", "s16le", "-ar", strconv.Itoa(rate), "-ac", "1", "-i", "pipe:0",
		"-c:a", "libopus", "-b:a", opusBitrate, outPath)
	cmd.Stdin = bytes.NewReader(pcm)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.ErrorContext(ctx, "voice: ffmpeg opus encode failed", "exit", exitStatus(ctx, err), "output_bytes", len(out))
		return fmt.Errorf("gemini tts: %w: exit %s", ErrEncode, exitStatus(ctx, err))
	}
	return nil
}
