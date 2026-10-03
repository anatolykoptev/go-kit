package voice

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

// Provider names accepted in [Settings.Provider] and [Settings.Fallback].
const (
	ProviderEdge   = "edge"
	ProviderGemini = "gemini"
	FallbackNone   = "none"
)

// Settings is the provider-selection input, decoupled from any config format.
type Settings struct {
	Provider      string // "" or "edge" (default) | "gemini"
	Fallback      string // "" or "edge" (default) | "none"; used only with a non-edge Provider
	EdgeVoice     string
	GeminiAPIKey  string // supplied by the caller; the library never reads the environment
	GeminiModel   string
	GeminiVoice   string
	GeminiStyle   string
	GeminiBaseURL string // tests only
	// OutputDir is where audio files are created by every provider; empty
	// means the OS temp dir.
	OutputDir string
	// Logger receives operational logs; nil means slog.Default().
	Logger *slog.Logger
}

// NewProviderFromSettings builds the configured provider. An empty or unknown
// provider yields plain edge-tts. Gemini is wrapped so edge-tts takes over
// whenever Gemini is unavailable or fails, unless Fallback is "none".
func NewProviderFromSettings(s Settings) Provider {
	log := loggerOrDefault(s.Logger)
	edge := NewEdgeProvider(EdgeConfig{Voice: s.EdgeVoice, OutputDir: s.OutputDir, Logger: log})
	switch strings.ToLower(strings.TrimSpace(s.Provider)) {
	case "", ProviderEdge:
		return edge
	case ProviderGemini:
		gemini := NewGeminiProvider(GeminiConfig{
			APIKey:    s.GeminiAPIKey,
			Model:     s.GeminiModel,
			Voice:     s.GeminiVoice,
			Style:     s.GeminiStyle,
			BaseURL:   s.GeminiBaseURL,
			OutputDir: s.OutputDir,
			Logger:    log,
		})
		if s.GeminiAPIKey == "" {
			log.Warn("voice: gemini api key not configured; gemini tts disabled")
		}
		if strings.EqualFold(strings.TrimSpace(s.Fallback), FallbackNone) {
			return gemini
		}
		return NewFallbackProvider("gemini", gemini, edge, log)
	default:
		log.Warn("voice: unknown tts provider, using edge", "provider", s.Provider)
		return edge
	}
}

// EdgeReserve is the slice of the caller's deadline that [FallbackProvider]
// keeps for the fallback: the primary runs under (deadline - EdgeReserve).
// Sized from measurements of edge-tts with a Russian neural voice on a 4-core
// ARM host: 54.7 s for 2000 runes (+1.2 s ffmpeg) and 100.2 s for 4000 runes
// (+4.4 s ffmpeg); a 2000-rune Gemini request took ~35 s. With a 120 s caller
// deadline, 60 s covers an edge run for the 2000 runes Gemini accepts; longer
// text skips Gemini and edge gets the whole deadline.
//
// The reserve applies only inside FallbackProvider: a bare [GeminiProvider]
// gets the caller's full deadline.
const EdgeReserve = 60 * time.Second

// FallbackProvider tries primary and, when it is unavailable or fails,
// synthesizes with fallback instead.
type FallbackProvider struct {
	name     string // primary's label for logs
	primary  Provider
	fallback Provider
	log      *slog.Logger
}

// NewFallbackProvider returns a FallbackProvider. name labels the primary in
// logs; a nil logger means slog.Default().
func NewFallbackProvider(name string, primary, fallback Provider, logger *slog.Logger) *FallbackProvider {
	return &FallbackProvider{name: name, primary: primary, fallback: fallback, log: loggerOrDefault(logger)}
}

// IsAvailable is true when either provider can synthesize.
func (p *FallbackProvider) IsAvailable() bool {
	return p.primary.IsAvailable() || p.fallback.IsAvailable()
}

// Synthesize implements [Provider]. The fallback never runs once the caller's
// ctx is done: a second synthesis could not finish.
func (p *FallbackProvider) Synthesize(ctx context.Context, text, lang string) (string, error) {
	var primaryErr error
	if p.primary.IsAvailable() {
		path, stop, err := p.tryPrimary(ctx, text, lang)
		if err == nil {
			return path, nil
		}
		if stop || !p.fallback.IsAvailable() {
			return "", err
		}
		primaryErr = err
	}
	path, err := p.fallback.Synthesize(ctx, text, lang)
	if err != nil && primaryErr != nil {
		return "", errors.Join(primaryErr, err)
	}
	return path, err
}

// tryPrimary runs the primary under (deadline - EdgeReserve) so the fallback
// keeps its reserve; ctx itself (not the trimmed one) still gates whether the
// fallback runs. stop is true when the caller's ctx is done.
func (p *FallbackProvider) tryPrimary(ctx context.Context, text, lang string) (path string, stop bool, err error) {
	pctx := ctx
	if dl, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		pctx, cancel = context.WithDeadline(ctx, dl.Add(-EdgeReserve))
		defer cancel()
	}
	path, err = p.primary.Synthesize(pctx, text, lang)
	if err == nil {
		return path, false, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		kind := "canceled"
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			kind = "parent_deadline"
		}
		p.log.Warn("voice: primary tts failed and caller context ended, fallback skipped",
			"provider", p.name, "kind", kind)
		return "", true, err
	}
	args := []any{"provider", p.name, "kind", errKind(err)}
	var httpErr *GeminiHTTPError
	if errors.As(err, &httpErr) {
		args = append(args, "status", httpErr.Status)
		if httpErr.Reason != "" {
			args = append(args, "reason", httpErr.Reason)
		}
	}
	p.log.Warn("voice: primary tts failed, falling back", args...)
	return "", false, err
}

// errKind labels an error for logs without echoing its text.
func errKind(err error) string {
	var httpErr *GeminiHTTPError
	switch {
	case errors.As(err, &httpErr):
		return "http"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrTextTooLong):
		return "text_too_long"
	case errors.Is(err, ErrBudgetExhausted):
		return "budget_exhausted"
	default:
		return "error"
	}
}
