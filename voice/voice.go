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

// Synthesize implements [Provider]. lang "en"/"english" and "ru"/"russian"
// select a built-in voice; any other value keeps the configured one.
func (p *EdgeProvider) Synthesize(ctx context.Context, text, lang string) (string, error) {
	voiceName := p.voice
	switch lang {
	case "en", "english":
		voiceName = enVoice
	case "ru", "russian":
		voiceName = defaultEdgeVoice
	}
	p.log.InfoContext(ctx, "voice: synthesizing", "provider", "edge",
		"voice", voiceName, "lang", lang, "text_runes", utf8.RuneCountInString(text))

	outPath, err := createOutputFile(p.outputDir, "edge tts")
	if err != nil {
		return "", err
	}
	// edge-tts writes mp3; ffmpeg converts it to ogg/opus.
	mp3Path := outPath + ".mp3"
	defer os.Remove(mp3Path)

	// The text is in argv by edge-tts's CLI contract (visible to `ps`).
	cmd := exec.CommandContext(ctx, p.edgeBin, //nolint:gosec // configured binary, argv slice, no shell
		"--voice", voiceName, "--text", text, "--write-media", mp3Path)
	if out, err := cmd.CombinedOutput(); err != nil {
		// The CLI output can echo the input text: log its size only.
		p.log.ErrorContext(ctx, "voice: edge-tts failed", "exit", exitStatus(err), "output_bytes", len(out))
		os.Remove(outPath)
		return "", fmt.Errorf("edge-tts failed: exit %s", exitStatus(err))
	}

	ff := exec.CommandContext(ctx, p.ffmpegBin, //nolint:gosec // configured binary, argv slice, no shell
		"-y", "-i", mp3Path, "-c:a", "libopus", "-b:a", opusBitrate, outPath)
	if out, err := ff.CombinedOutput(); err != nil {
		p.log.ErrorContext(ctx, "voice: ffmpeg ogg conversion failed",
			"exit", exitStatus(err), "output_bytes", len(out))
		// Keep the raw mp3 rather than fail the reply.
		if err := os.Rename(mp3Path, outPath); err != nil {
			os.Remove(outPath)
			return "", fmt.Errorf("edge-tts: keep mp3: %w", err)
		}
	}
	p.log.InfoContext(ctx, "voice: speech synthesized", "provider", "edge")
	return outPath, nil
}

// IsAvailable reports whether the edge-tts executable is on PATH.
func (p *EdgeProvider) IsAvailable() bool {
	_, err := exec.LookPath(p.edgeBin)
	return err == nil
}

// exitStatus renders a command error without any command output.
func exitStatus(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return strconv.Itoa(ee.ExitCode())
	}
	return "start_failed"
}

// createOutputFile makes a unique, empty tts_*.ogg file in dir (created if
// missing; empty means the OS temp dir) and returns its path.
func createOutputFile(dir, who string) (string, error) {
	if dir != "" {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return "", fmt.Errorf("%s: output dir: %w", who, err)
		}
		dir = filepath.Clean(dir)
	}
	f, err := os.CreateTemp(dir, "tts_*.ogg")
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
