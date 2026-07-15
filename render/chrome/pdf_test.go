package chrome_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/render"
	"github.com/anatolykoptev/go-kit/render/chrome"
)

func cdpURLFromEnv(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration in -short mode")
	}
	u := os.Getenv("CDPURL")
	if u == "" {
		t.Skip("CDPURL not set; skipping CDP integration test")
	}
	return u
}

func TestWithHeaderText(t *testing.T) {
	o := chrome.ApplyPDFOptions(chrome.PDFOptions{}, chrome.WithHeaderText("DRAFT"))
	if o.HeaderText != "DRAFT" {
		t.Errorf("HeaderText = %q, want DRAFT", o.HeaderText)
	}
}

func TestWithoutHeader(t *testing.T) {
	o := chrome.ApplyPDFOptions(chrome.PDFOptions{HeaderText: "X"}, chrome.WithoutHeader())
	if o.HeaderText != "" {
		t.Errorf("HeaderText = %q, want empty", o.HeaderText)
	}
}

func TestWithPageNumbers_False(t *testing.T) {
	o := chrome.ApplyPDFOptions(chrome.PDFOptions{}, chrome.WithPageNumbers(false))
	if o.FooterTemplate != `<div></div>` {
		t.Errorf("FooterTemplate = %q, want empty div", o.FooterTemplate)
	}
}

func TestWithFooterTemplate_Custom(t *testing.T) {
	custom := `<div>custom</div>`
	o := chrome.ApplyPDFOptions(chrome.PDFOptions{}, chrome.WithFooterTemplate(custom))
	if o.FooterTemplate != custom {
		t.Errorf("FooterTemplate = %q, want %q", o.FooterTemplate, custom)
	}
}

func TestRenderPDF_CDPURLRequired(t *testing.T) {
	_, err := chrome.RenderPDF(context.Background(), "<html></html>", chrome.PDFOptions{})
	if err == nil {
		t.Fatal("expected error when CDPURL is empty, got nil")
	}
	if !strings.Contains(err.Error(), "CDPURL") {
		t.Errorf("expected error to mention CDPURL, got: %v", err)
	}
}

func TestRenderPDF_UnreachableCDP(t *testing.T) {
	const badURL = "http://127.0.0.1:1"
	_, err := chrome.RenderPDF(context.Background(), "<html></html>", chrome.PDFOptions{
		CDPURL:  badURL,
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for unreachable CDP endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected error to mention %q, got: %v", "unreachable", err)
	}
	if !strings.Contains(err.Error(), badURL) {
		t.Errorf("expected error to mention %q, got: %v", badURL, err)
	}
}

func TestRenderPDF_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	_, err := chrome.RenderPDF(ctx, "<html></html>", chrome.PDFOptions{
		CDPURL:  "http://127.0.0.1:1",
		Timeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected non-nil error for 1ns context deadline")
	}
	// With the eager CDP probe (Fix 1), a 1ns deadline is exceeded during
	// the probe's http.Do. The returned error wraps context.DeadlineExceeded.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestRenderPDF_Integration(t *testing.T) {
	url := cdpURLFromEnv(t)
	pdf, err := chrome.RenderPDF(context.Background(),
		"<html><body><h1>Hello</h1></body></html>",
		chrome.PDFOptions{CDPURL: url, Timeout: 30 * time.Second},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		tail := len(pdf)
		if tail > 16 {
			tail = 16
		}
		t.Errorf("expected PDF magic prefix, got first bytes: %q", pdf[:tail])
	}
	if len(pdf) <= 500 {
		t.Errorf("expected PDF > 500 bytes, got %d", len(pdf))
	}
}

// TestRenderPDF_NoDuplicateHeaderFooterIntegration ensures that the default
// rendering does not produce duplicate CONFIDENTIAL / page-number watermarks
// (bug: CSS @page content rules + Chrome header template both rendered).
func TestRenderPDF_NoDuplicateHeaderFooterIntegration(t *testing.T) {
	cdpURL := cdpURLFromEnv(t)

	ctx := context.Background()
	html := "<html><body><h1>T</h1></body></html>"
	pdfBytes, err := chrome.RenderPDF(ctx, html, chrome.PDFOptions{CDPURL: cdpURL})
	if err != nil {
		t.Fatalf("RenderPDF: %v", err)
	}
	// Since we removed CSS @page content rules, and the default HeaderText is
	// now empty, the output must not contain "CONFIDENTIAL" text anywhere.
	// Note: PDF text extraction is brittle; we check that the raw byte stream
	// does not contain the substring. This is a coarse but effective guard:
	// CSS-injected text appears encoded in the PDF content stream.
	if bytes.Count(pdfBytes, []byte("CONFIDENTIAL")) > 0 {
		t.Errorf("expected no 'CONFIDENTIAL' watermark in output; found %d",
			bytes.Count(pdfBytes, []byte("CONFIDENTIAL")))
	}
}

func TestRenderMarkdownToPDF_Integration(t *testing.T) {
	url := cdpURLFromEnv(t)
	pdf, err := chrome.RenderMarkdownToPDF(context.Background(),
		"# Test\n\ntext",
		chrome.CombinedOptions{
			PDF: chrome.PDFOptions{CDPURL: url, Timeout: 30 * time.Second},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		tail := len(pdf)
		if tail > 16 {
			tail = 16
		}
		t.Errorf("expected PDF magic prefix, got first bytes: %q", pdf[:tail])
	}
}

func TestResolveImagePreset_UnknownName(t *testing.T) {
	_, err := chrome.ResolveImagePreset("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown preset name, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to mention preset name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "render: unknown image preset") {
		t.Errorf("expected 'render: unknown image preset' prefix, got: %v", err)
	}
}

func TestResolveImagePreset_KnownNames(t *testing.T) {
	for _, name := range []string{"og-image", "twitter-card", "square-1080"} {
		p, err := chrome.ResolveImagePreset(name)
		if err != nil {
			t.Errorf("ResolveImagePreset(%q): unexpected error: %v", name, err)
			continue
		}
		if p.WidthPx <= 0 || p.HeightPx <= 0 {
			t.Errorf("preset %q: dimensions must be positive, got %dx%d", name, p.WidthPx, p.HeightPx)
		}
		if p.PPI <= 0 {
			t.Errorf("preset %q: PPI must be positive, got %d", name, p.PPI)
		}
		if p.Theme == "" {
			t.Errorf("preset %q: Theme must not be empty", name)
		}
	}
}

func TestRenderImage_Integration(t *testing.T) {
	url := cdpURLFromEnv(t)
	img, err := chrome.RenderImage(context.Background(),
		"<html><body><h1>Image</h1></body></html>",
		chrome.ImageOptions{
			CDPURL:            url,
			Format:            chrome.ImageFormatPNG,
			Width:             800,
			Height:            600,
			DeviceScaleFactor: 1.0,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// PNG magic bytes
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(img) < 8 || !bytes.Equal(img[:8], pngMagic) {
		t.Errorf("expected PNG magic, got first bytes: % x", img[:min(8, len(img))])
	}
}

func TestRenderMarkdownToImage_Integration(t *testing.T) {
	url := cdpURLFromEnv(t)
	img, err := chrome.RenderMarkdownToImage(context.Background(),
		"# Title\n\nHello world.",
		chrome.CombinedImageOptions{
			HTML: render.Options{},
			Image: chrome.ImageOptions{
				CDPURL:            url,
				Format:            chrome.ImageFormatPNG,
				Width:             800,
				Height:            600,
				DeviceScaleFactor: 1.0,
			},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(img) < 8 || !bytes.Equal(img[:8], pngMagic) {
		t.Errorf("expected PNG magic, got first bytes: % x", img[:min(8, len(img))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
