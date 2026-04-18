package fileopt

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"testing"
)

// tinyRedPNG is a small but fully valid PNG generated at init via stdlib.
// A hand-rolled byte literal was previously used and failed libpng CRC checks
// in cwebp/oxipng 1.3+ — encoding through image/png guarantees correctness.
var tinyRedPNG = mustEncodeTestPNG()

func mustEncodeTestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestOptimizePNG_MissingBinaryReturnsOriginal(t *testing.T) {
	t.Setenv("FILEOPT_OXIPNG_PATH", "/nonexistent/oxipng-xyz")
	got, err := OptimizePNG(context.Background(), tinyRedPNG)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !bytes.Equal(got, tinyRedPNG) {
		t.Fatalf("expected original bytes returned when binary missing")
	}
}

func TestOptimizeWebP_MissingBinaryReturnsOriginal(t *testing.T) {
	t.Setenv("FILEOPT_CWEBP_PATH", "/nonexistent/cwebp-xyz")
	got, err := OptimizeWebP(context.Background(), tinyRedPNG, 80)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !bytes.Equal(got, tinyRedPNG) {
		t.Fatalf("expected original bytes returned when binary missing")
	}
}

func TestOptimizePNG_Real(t *testing.T) {
	if _, err := exec.LookPath("oxipng"); err != nil {
		t.Skip("oxipng not on PATH")
	}
	got, err := OptimizePNG(context.Background(), tinyRedPNG)
	if err != nil {
		t.Fatalf("OptimizePNG: %v", err)
	}
	if len(got) == 0 || !bytes.HasPrefix(got, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("expected valid PNG output, got %v", got[:min(8, len(got))])
	}
}

func TestOptimizeWebP_Real(t *testing.T) {
	if _, err := exec.LookPath("cwebp"); err != nil {
		t.Skip("cwebp not on PATH")
	}
	got, err := OptimizeWebP(context.Background(), tinyRedPNG, 80)
	if err != nil {
		t.Fatalf("OptimizeWebP: %v", err)
	}
	// WebP magic: "RIFF....WEBP"
	if len(got) < 12 || !bytes.Equal(got[:4], []byte("RIFF")) || !bytes.Equal(got[8:12], []byte("WEBP")) {
		t.Fatalf("expected valid WebP output")
	}
}

func TestOptimizeWebP_QualityValidated(t *testing.T) {
	_, err := OptimizeWebP(context.Background(), tinyRedPNG, 200)
	if err == nil {
		t.Fatalf("expected error on quality > 100, got nil")
	}
}
