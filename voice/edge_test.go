package voice

import (
	"context"
	"os"
	"path/filepath"
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

func TestEdge_FFmpegFailureKeepsMP3WithoutLeaking(t *testing.T) {
	p, dir, buf := newShimEdge(t, edgeOK, `echo "ffmpeg says: $*"; exit 1`)

	path, err := p.Synthesize(context.Background(), secretText, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "mp3-bytes" {
		t.Errorf("content = %q, want the raw mp3", got)
	}
	if n := len(dirEntries(t, dir)); n != 1 {
		t.Errorf("output dir holds %v", dirEntries(t, dir))
	}
	if strings.Contains(buf.String(), "ffmpeg says") || !strings.Contains(buf.String(), "output_bytes=") {
		t.Errorf("ffmpeg output logged: %s", buf.String())
	}
}

func TestEdge_IsAvailable(t *testing.T) {
	p := NewEdgeProvider(EdgeConfig{})
	p.edgeBin = filepath.Join(t.TempDir(), "missing")
	if p.IsAvailable() {
		t.Error("missing binary reported available")
	}
	p.edgeBin = writeShim(t, "edge-tts", "exit 0")
	if !p.IsAvailable() {
		t.Error("present binary reported unavailable")
	}
}
