package voice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	defaultEdgeVoice = "ru-RU-DmitryNeural"
	enVoice          = "en-US-GuyNeural"
	outFileMode      = 0o644 // CreateTemp makes 0600; the files are handed to other processes
	dirMode          = 0o755
	opusBitrate      = "64k"
)

// Provider converts text to speech audio.
type Provider interface {
	// Synthesize writes the speech for text to a new ogg/opus file and returns
	// its path. The caller owns (and removes) the file. lang is a hint such as
	// "ru" or "en"; providers may ignore it.
	Synthesize(ctx context.Context, text, lang string) (audioPath string, err error)
	// IsAvailable reports whether the provider can synthesize right now. It
	// does not probe the network.
	IsAvailable() bool
}

// EdgeConfig configures [EdgeProvider].
type EdgeConfig struct {
	// Voice is the default edge-tts voice, e.g. "ru-RU-DmitryNeural". Empty
	// means ru-RU-DmitryNeural.
	Voice string
	// OutputDir is where audio files are created (made if missing). Empty
	// means the OS temp dir.
	OutputDir string
	// Logger receives operational logs; nil means slog.Default().
	Logger *slog.Logger
}

// EdgeProvider uses Microsoft's edge-tts CLI (free, no API key).
type EdgeProvider struct {
	voice     string
	outputDir string
	log       *slog.Logger
	// edgeBin and ffmpegBin are the executables; fields so tests can point at
	// shims.
	edgeBin   string
	ffmpegBin string
}

// NewEdgeProvider returns an EdgeProvider.
func NewEdgeProvider(cfg EdgeConfig) *EdgeProvider {
	if cfg.Voice == "" {
		cfg.Voice = defaultEdgeVoice
	}
	return &EdgeProvider{
		voice:     cfg.Voice,
		outputDir: cfg.OutputDir,
		log:       loggerOrDefault(cfg.Logger),
		edgeBin:   "edge-tts",
		ffmpegBin: "ffmpeg",
	}
}

// ErrEncode means ffmpeg could not produce the ogg/opus file.
var ErrEncode = errors.New("voice: ffmpeg encode failed")

// pickVoice applies the language hint: "en"/"english" and "ru"/"russian" select
// a built-in voice unless the configured voice already speaks that language
// (its locale prefix, e.g. "ru" in "ru-RU-SvetlanaNeural", matches the hint).
// Any other hint keeps the configured voice.
func pickVoice(configured, lang string) string {
	var code, builtin string
	switch lang {
	case "en", "english":
		code, builtin = "en", enVoice
	case "ru", "russian":
		code, builtin = "ru", defaultEdgeVoice
	default:
		return configured
	}
	if prefix, _, _ := strings.Cut(configured, "-"); strings.EqualFold(prefix, code) {
		return configured
	}
	return builtin
}

// Synthesize implements [Provider]. A "ru"/"en" lang hint selects a built-in
// voice only when the configured voice speaks another language.
func (p *EdgeProvider) Synthesize(ctx context.Context, text, lang string) (string, error) {
	voiceName := pickVoice(p.voice, lang)
	p.log.InfoContext(ctx, "voice: synthesizing", "provider", "edge",
		"voice", voiceName, "lang", lang, "text_runes", utf8.RuneCountInString(text))

	outPath, err := createTempFile(p.outputDir, "tts_*.ogg", "edge tts")
	if err != nil {
		return "", err
	}
	// edge-tts writes mp3; ffmpeg converts it to ogg/opus. The mp3 gets its own
	// random name so it cannot be predicted from the .ogg.
	mp3Path, err := createTempFile(p.outputDir, "tts_*.mp3", "edge tts")
	if err != nil {
		os.Remove(outPath)
		return "", err
	}
	defer os.Remove(mp3Path)

	// The text is in argv by edge-tts's CLI contract (visible to `ps`). The
	// "--text=" form keeps text that starts with "-" from being parsed as a flag.
	cmd := exec.CommandContext(ctx, p.edgeBin, //nolint:gosec // configured binary, argv slice, no shell
		"--voice", voiceName, "--text="+text, "--write-media", mp3Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		// The CLI output can echo the input text: log its size only.
		p.log.ErrorContext(ctx, "voice: edge-tts failed", "exit", exitStatus(ctx, err), "output_bytes", len(out))
		os.Remove(outPath)
		return "", fmt.Errorf("edge-tts failed: exit %s", exitStatus(ctx, err))
	}

	ff := exec.CommandContext(ctx, p.ffmpegBin, //nolint:gosec // configured binary, argv slice, no shell
		"-y", "-i", mp3Path, "-c:a", "libopus", "-b:a", opusBitrate, outPath)
	if out, err := ff.CombinedOutput(); err != nil {
		p.log.ErrorContext(ctx, "voice: ffmpeg ogg conversion failed",
			"exit", exitStatus(ctx, err), "output_bytes", len(out))
		os.Remove(outPath)
		return "", fmt.Errorf("edge tts: %w: exit %s", ErrEncode, exitStatus(ctx, err))
	}
	p.log.InfoContext(ctx, "voice: speech synthesized", "provider", "edge")
	return outPath, nil
}

// IsAvailable reports whether both edge-tts and ffmpeg are on PATH.
func (p *EdgeProvider) IsAvailable() bool {
	if _, err := exec.LookPath(p.edgeBin); err != nil {
		return false
	}
	_, err := exec.LookPath(p.ffmpegBin)
	return err == nil
}

// exitStatus renders a command error without any command output; "canceled"
// when the context ended (the process was killed by it).
func exitStatus(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "canceled"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return strconv.Itoa(ee.ExitCode())
	}
	return "start_failed"
}

// createTempFile makes a unique, empty file matching pattern in dir (created
// if missing; empty means the OS temp dir) and returns its path.
func createTempFile(dir, pattern, who string) (string, error) {
	if dir != "" {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return "", fmt.Errorf("%s: output dir: %w", who, err)
		}
		dir = filepath.Clean(dir)
	}
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("%s: temp file: %w", who, err)
	}
	path := f.Name()
	f.Close()
	if err := os.Chmod(path, outFileMode); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("%s: chmod: %w", who, err)
	}
	return path, nil
}

func loggerOrDefault(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}
