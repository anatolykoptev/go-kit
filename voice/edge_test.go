package voice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeShim writes an executable /bin/sh script and returns its path.
func writeShim(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// edgeOK emulates edge-tts: it writes "mp3-bytes" to the --write-media path.
const edgeOK = `while [ $# -gt 0 ]; do [ "$1" = "--write-media" ] && out="$2"; shift; done; printf mp3-bytes > "$out"`

// ffmpegOK emulates ffmpeg: it writes "ogg-bytes" to its last argument.
const ffmpegOK = `for a; do last="$a"; done; printf ogg-bytes > "$last"`

func newShimEdge(t *testing.T, edgeBody, ffmpegBody string) (*EdgeProvider, string, *syncBuf) {
	t.Helper()
	log, buf := captureLog()
	dir := filepath.Join(t.TempDir(), "out")
	p := NewEdgeProvider(EdgeConfig{OutputDir: dir, Logger: log})
	p.edgeBin = writeShim(t, "edge-tts", edgeBody)
	p.ffmpegBin = writeShim(t, "ffmpeg", ffmpegBody)
	return p, dir, buf
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestEdge_SynthesizeWritesOggInOutputDir(t *testing.T) {
	p, dir, buf := newShimEdge(t, edgeOK, ffmpegOK)

	a, err := p.Synthesize(context.Background(), secretText, "ru")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Synthesize(context.Background(), secretText, "en")
	if err != nil {
		t.Fatal(err)
	}
	if a == b || filepath.Dir(a) != dir || !strings.HasSuffix(a, ".ogg") {
		t.Fatalf("paths %q %q: want distinct .ogg files in %s", a, b, dir)
	}
	if got, _ := os.ReadFile(a); string(got) != "ogg-bytes" {
		t.Errorf("content = %q", got)
	}
	if n := len(dirEntries(t, dir)); n != 2 {
		t.Errorf("output dir holds %v, want exactly the two .ogg files (mp3 temp removed)", dirEntries(t, dir))
	}
	if strings.Contains(buf.String(), secretText) {
		t.Errorf("log leaks the text: %s", buf.String())
	}
}

// The CLI output can echo the conversation text, so only its size may be logged.
func TestEdge_FailureLogsExitAndLengthNotOutput(t *testing.T) {
	p, dir, buf := newShimEdge(t, `echo "boom: $*"; exit 3`, ffmpegOK)

	path, err := p.Synthesize(context.Background(), secretText, "ru")
	if err == nil || path != "" {
		t.Fatalf("path=%q err=%v, want failure", path, err)
	}
	dump := buf.String()
	if strings.Contains(dump, secretText) || strings.Contains(err.Error(), secretText) {
		t.Errorf("CLI output leaked: log=%s err=%v", dump, err)
	}
	if !strings.Contains(dump, "exit=3") || !strings.Contains(dump, "output_bytes=") {
		t.Errorf("exit status / output length not logged: %s", dump)
	}
	if n := len(dirEntries(t, dir)); n != 0 {
		t.Errorf("failed run left files behind: %v", dirEntries(t, dir))
	}
}

func TestEdge_FFmpegFailureIsAnErrorAndLeavesNothing(t *testing.T) {
	p, dir, buf := newShimEdge(t, edgeOK, `echo "ffmpeg says: $*"; exit 1`)

	path, err := p.Synthesize(context.Background(), secretText, "")
	if !errors.Is(err, ErrEncode) || path != "" {
		t.Fatalf("path=%q err=%v, want ErrEncode and no path", path, err)
	}
	if n := dirEntries(t, dir); len(n) != 0 {
		t.Errorf("failed encode left files behind: %v", n)
	}
	dump := buf.String()
	if strings.Contains(dump, "ffmpeg says") || strings.Contains(dump, secretText) ||
		strings.Contains(err.Error(), secretText) || !strings.Contains(dump, "output_bytes=") {
		t.Errorf("ffmpeg output or text leaked / length missing: log=%s err=%v", dump, err)
	}
}

func TestEdge_CanceledContextReportsCanceled(t *testing.T) {
	p, _, buf := newShimEdge(t, `exit 1`, ffmpegOK)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Synthesize(ctx, "hi", ""); err == nil || !strings.Contains(buf.String(), "exit=canceled") {
		t.Errorf("err=%v log=%s, want exit=canceled", err, buf.String())
	}
}

// argShim records its argv (one per line) to the returned file, then behaves
// like edgeOK.
func argShim(t *testing.T) (body, argsFile string) {
	t.Helper()
	argsFile = filepath.Join(t.TempDir(), "args")
	return `printf '%s\n' "$@" > '` + argsFile + `'; ` + edgeOK, argsFile
}

func readArgs(t *testing.T, file string) []string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestEdge_TextStartingWithDashIsNotAFlag(t *testing.T) {
	body, argsFile := argShim(t)
	p, _, _ := newShimEdge(t, body, ffmpegOK)
	text := "-Привет"
	path, err := p.Synthesize(context.Background(), text, "")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	args := readArgs(t, argsFile)
	if !slices.Contains(args, "--text="+text) || slices.Contains(args, "--text") || slices.Contains(args, text) {
		t.Errorf("argv = %q, want a single --text=<text> argument", args)
	}
}

func TestEdge_IntermediateMP3NameIsNotDerivedFromOutput(t *testing.T) {
	body, argsFile := argShim(t)
	p, _, _ := newShimEdge(t, body, ffmpegOK)
	path, err := p.Synthesize(context.Background(), "hi", "")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	mp3 := argAfter(readArgs(t, argsFile), "--write-media")
	if !strings.HasSuffix(mp3, ".mp3") || mp3 == path+".mp3" {
		t.Errorf("mp3 path %q is predictable from the output path %q", mp3, path)
	}
}

func TestEdge_LanguageHintKeepsMatchingConfiguredVoice(t *testing.T) {
	cases := []struct{ configured, lang, want string }{
		{"ru-RU-SvetlanaNeural", "ru", "ru-RU-SvetlanaNeural"},
		{"ru-RU-SvetlanaNeural", "russian", "ru-RU-SvetlanaNeural"},
		{"en-GB-RyanNeural", "en", "en-GB-RyanNeural"},
		{"ru-RU-SvetlanaNeural", "en", "en-US-GuyNeural"},
		{"en-GB-RyanNeural", "ru", "ru-RU-DmitryNeural"},
		{"de-DE-ConradNeural", "fr", "de-DE-ConradNeural"}, // unknown hint
		{"de-DE-ConradNeural", "", "de-DE-ConradNeural"},
	}
	for _, c := range cases {
		t.Run(c.configured+"/"+c.lang, func(t *testing.T) {
			body, argsFile := argShim(t)
			p, _, _ := newShimEdge(t, body, ffmpegOK)
			p.voice = c.configured
			path, err := p.Synthesize(context.Background(), "hi", c.lang)
			if err != nil {
				t.Fatal(err)
			}
			os.Remove(path)
			if got := argAfter(readArgs(t, argsFile), "--voice"); got != c.want {
				t.Errorf("voice = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEdge_IsAvailableNeedsBothBinaries(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	present := func(n string) string { return writeShim(t, n, "exit 0") }
	cases := []struct {
		name, edge, ffmpeg string
		want               bool
	}{
		{"both", present("edge-tts"), present("ffmpeg"), true},
		{"no edge", missing, present("ffmpeg"), false},
		{"no ffmpeg", present("edge-tts"), missing, false},
	}
	for _, c := range cases {
		p := NewEdgeProvider(EdgeConfig{})
		p.edgeBin, p.ffmpegBin = c.edge, c.ffmpeg
		if got := p.IsAvailable(); got != c.want {
			t.Errorf("%s: IsAvailable = %v, want %v", c.name, got, c.want)
		}
	}
}
