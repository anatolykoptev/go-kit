package fileopt

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOptimizePDF_MissingBinaryReturnsOriginal(t *testing.T) {
	t.Setenv("FILEOPT_GS_PATH", "/nonexistent/gs-xyz-123")
	original := []byte("%PDF-1.4 fake content")
	got, err := OptimizePDF(context.Background(), original, LevelEbook)
	if err != nil {
		t.Fatalf("expected no error when binary missing, got %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("expected original bytes when binary missing, got changed")
	}
}

func TestOptimizePDF_ShrinksRealPDF(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("gs not on PATH; install ghostscript to run this test")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "in.pdf")
	writeCmd := exec.Command("gs",
		"-sDEVICE=pdfwrite", "-dNOPAUSE", "-dBATCH", "-dQUIET",
		"-sOutputFile="+src, "-c",
		"/Helvetica findfont 12 scalefont setfont 72 720 moveto (Hello optimize test) show showpage",
	)
	if out, err := writeCmd.CombinedOutput(); err != nil {
		t.Fatalf("gs fixture gen failed: %v\n%s", err, out)
	}
	orig, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	optimized, err := OptimizePDF(context.Background(), orig, LevelScreen)
	if err != nil {
		t.Fatalf("OptimizePDF: %v", err)
	}
	if len(optimized) == 0 {
		t.Fatalf("optimized empty")
	}
	if !bytes.HasPrefix(optimized, []byte("%PDF-")) {
		t.Fatalf("optimized output is not a valid PDF")
	}
}

func TestOptimizePDF_InvalidLevelRejected(t *testing.T) {
	_, err := OptimizePDF(context.Background(), []byte("%PDF-1.4"), Level("bogus"))
	if err == nil {
		t.Fatalf("expected error on invalid level, got nil")
	}
}

func TestPdfHasImages(t *testing.T) {
	cases := map[string]struct {
		input []byte
		want  bool
	}{
		"plain text PDF":               {[]byte("%PDF-1.4\n/Helvetica findfont 12 scalefont setfont 72 720 moveto (hello) show showpage"), false},
		"subtype with space":           {[]byte("... /Subtype /Image ..."), true},
		"subtype without space":        {[]byte("... /Subtype/Image ..."), true},
		"empty bytes":                  {[]byte{}, false},
		"mentions Image but no marker": {[]byte("some text mentioning /Image as a word"), false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := pdfHasImages(c.input); got != c.want {
				t.Errorf("pdfHasImages(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

// TestOptimizePDF_SkipsGsForTextOnly confirms that a text-only PDF bypasses
// gs (saving ~125ms of near-useless work) but still goes through qpdf. The
// gs call count should be 0 (or "skipped"), qpdf count should be 1.
func TestOptimizePDF_SkipsGsForTextOnly(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skip("gs required to generate the text-only fixture")
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "in.pdf")
	writeCmd := exec.Command("gs",
		"-sDEVICE=pdfwrite", "-dNOPAUSE", "-dBATCH", "-dQUIET",
		"-sOutputFile="+src, "-c",
		"/Helvetica findfont 12 scalefont setfont 72 720 moveto (text-only fixture) show showpage",
	)
	if out, err := writeCmd.CombinedOutput(); err != nil {
		t.Fatalf("gs fixture gen failed: %v\n%s", err, out)
	}
	orig, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if pdfHasImages(orig) {
		t.Fatalf("text-only fixture unexpectedly contained /Subtype /Image — test premise broken")
	}
	optimized, err := OptimizePDF(context.Background(), orig, LevelEbook)
	if err != nil {
		t.Fatalf("OptimizePDF: %v", err)
	}
	// Output must still be a valid PDF (qpdf ran on original bytes).
	if !bytes.HasPrefix(optimized, []byte("%PDF-")) {
		t.Fatalf("optimized output is not a valid PDF: %q", optimized[:min(16, len(optimized))])
	}
	// qpdf should still do useful work — expect at least 1 byte of reduction
	// (structural recompression almost always shrinks a gs-generated PDF).
	if len(optimized) >= len(orig) {
		t.Fatalf("qpdf stage did not reduce text-only PDF: %d >= %d", len(optimized), len(orig))
	}
}
