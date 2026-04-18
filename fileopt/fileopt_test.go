package fileopt

import (
	"context"
	"testing"
)

func TestKindFromExt(t *testing.T) {
	cases := map[string]Kind{
		".pdf":  KindPDF,
		".PDF":  KindPDF,
		"pdf":   KindPDF, // bare extension — no leading dot
		".png":  KindPNG,
		".webp": KindWebP,
		".jpg":  KindUnsupported,
		".gif":  KindUnsupported,
	}
	for ext, want := range cases {
		if got := KindFromExt(ext); got != want {
			t.Errorf("KindFromExt(%q) = %v, want %v", ext, got, want)
		}
	}
}

func TestOptimizeBytes_Unsupported(t *testing.T) {
	data := []byte("hello")
	got, err := OptimizeBytes(context.Background(), data, KindUnsupported, LevelEbook, 80)
	if err != nil {
		t.Fatalf("expected no error for unsupported kind (passthrough), got %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestRatio(t *testing.T) {
	if r := Ratio(100, 50); r != 0.5 {
		t.Errorf("Ratio(100, 50) = %v, want 0.5", r)
	}
	if r := Ratio(0, 10); r != 0 {
		t.Errorf("Ratio(0, 10) = %v, want 0 (guard against div-by-zero)", r)
	}
	if r := Ratio(100, 150); r != 1.5 {
		t.Errorf("Ratio(100, 150) = %v, want 1.5 (growth case)", r)
	}
}
