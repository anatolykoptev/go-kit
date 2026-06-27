package typst_test

import (
	"bytes"
	"context"
	"flag"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
	"github.com/anatolykoptev/go-kit/render/chrome"
	renderTypst "github.com/anatolykoptev/go-kit/render/typst"
)

var updateGolden = flag.Bool("update", false, "regenerate golden PNG files under testdata/render_image/golden")

// realisticMarkdownSample is the production trace that exposed the
// pandoc/typst incompatibility (exercises HR, blockquote, cite, links, lists,
// fenced code, tables, footnotes). Shared by golden tests in this external
// test file (internal sanitizer unit tests live in typst_pandoc_compat_test.go).
const realisticMarkdownSample = "# Quarterly Report\n\n" +
	"## Highlights\n\n" +
	"Revenue grew **37%** YoY with *strong* execution in EMEA and APAC.\n\n" +
	"---\n\n" +
	"> \"We are entering the next phase of growth.\"\n" +
	"> — CEO\n\n" +
	"Key achievements:\n" +
	"- Shipped 3 major features\n" +
	"- Reduced p99 latency by 40%\n" +
	"- Added 200+ enterprise customers\n\n" +
	"1. Customer onboarding revamp\n" +
	"2. New analytics dashboard\n" +
	"3. SOC2 Type II completion\n\n" +
	"See [docs](https://example.com) for details. As shown in [@koptev2026], the market is expanding.\n\n" +
	"```python\n" +
	"def revenue(growth_pct):\n" +
	"    return baseline * (1 + growth_pct)\n" +
	"```\n\n" +
	"| Region | Q1   | Q2   |\n" +
	"|--------|------|------|\n" +
	"| EMEA   | 1.2M | 1.7M |\n" +
	"| APAC   | 0.8M | 1.4M |\n\n" +
	"Footnote anchor[^1] for follow-up.\n\n" +
	"[^1]: Detailed methodology in appendix.\n"

// pngMagic is the 8-byte PNG signature.
var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func skipIfNoTypstPandoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst binary not on PATH")
	}
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc binary not on PATH")
	}
}

func TestTypstRenderImage_PNGMagic(t *testing.T) {
	skipIfNoTypstPandoc(t)
	r := renderTypst.NewTypstRenderer()
	out, err := r.RenderImage(context.Background(), "# Hello\n\nWorld", "markdown", render.Options{
		Theme:  "card",
		Width:  600,
		Height: 400,
		PPI:    144,
	})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	if len(out) < 8 || !bytes.Equal(out[:8], pngMagic) {
		t.Fatalf("output is not PNG (first 8 bytes: % x)", out[:min(8, len(out))])
	}
}

func TestTypstRenderImage_RespectsGeometry(t *testing.T) {
	skipIfNoTypstPandoc(t)
	r := renderTypst.NewTypstRenderer()
	const wantW, wantH = 600, 400
	out, err := r.RenderImage(context.Background(), "# Hello\n\nGeometry test.", "markdown", render.Options{
		Theme:  "card",
		Width:  wantW,
		Height: wantH,
		PPI:    72,
	})
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("png.DecodeConfig: %v", err)
	}
	if !within5Pct(cfg.Width, wantW) || !within5Pct(cfg.Height, wantH) {
		t.Fatalf("geometry off: got %dx%d want ~%dx%d", cfg.Width, cfg.Height, wantW, wantH)
	}
}

func TestTypstRenderImage_MultiPageError(t *testing.T) {
	skipIfNoTypstPandoc(t)
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("Paragraph content with enough text to flow onto more than one page when the page is small.\n\n")
	}
	r := renderTypst.NewTypstRenderer()
	_, err := r.RenderImage(context.Background(), b.String(), "markdown", render.Options{
		Theme:  "report",
		Width:  200,
		Height: 100,
		PPI:    144,
	})
	if err == nil {
		t.Fatal("expected multi-page error, got nil")
	}
	if !strings.Contains(err.Error(), "typst rendered multiple pages but image output supports 1") {
		t.Fatalf("wrong error message: %v", err)
	}
}

func TestTypstRenderImage_GoldenPresets(t *testing.T) {
	skipIfNoTypstPandoc(t)
	const md = "# Test card\n\nA simple example."
	cases := []string{"og-image", "twitter-card", "square-1080"}
	r := renderTypst.NewTypstRenderer()
	goldenDir := filepath.Join("testdata", "render_image", "golden")

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			preset, err := chrome.ResolveImagePreset(name)
			if err != nil {
				t.Fatalf("ResolveImagePreset(%q): %v", name, err)
			}
			out, err := r.RenderImage(context.Background(), md, "markdown", render.Options{
				Theme:  preset.Theme,
				Width:  preset.WidthPx,
				Height: preset.HeightPx,
				PPI:    preset.PPI,
			})
			if err != nil {
				t.Fatalf("RenderImage(%s): %v", name, err)
			}
			path := filepath.Join(goldenDir, name+".png")
			if *updateGolden {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("mkdir golden: %v", err)
				}
				if err := os.WriteFile(path, out, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s (%d bytes)", path, len(out))
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
			}
			if !bytes.Equal(out, want) {
				t.Fatalf("golden mismatch for %s: got %d bytes, want %d", name, len(out), len(want))
			}
		})
	}
}

func within5Pct(got, want int) bool {
	if want == 0 {
		return got == 0
	}
	return math.Abs(float64(got-want))/float64(want) <= 0.05
}

// TestTypstRenderImage_GoldenRealisticContent renders the realistic sample
// through the square-1080 preset and compares to a committed golden PNG.
// Run with `go test -update` to regenerate after intentional changes.
func TestTypstRenderImage_GoldenRealisticContent(t *testing.T) {
	skipIfNoTypstPandoc(t)
	preset, err := chrome.ResolveImagePreset("square-1080")
	if err != nil {
		t.Fatalf("ResolveImagePreset(square-1080): %v", err)
	}
	// Use a trimmed sample for the golden — it still exercises every
	// pandoc-ism (HR, blockquote, cite, links) but fits the square preset
	// without spilling onto a second page.
	const goldenSample = "# Quarterly Report\n\n" +
		"Revenue grew **37%** YoY.\n\n" +
		"---\n\n" +
		"> \"We are entering the next phase of growth.\"\n\n" +
		"See [docs](https://example.com); ref [@koptev2026].\n"
	r := renderTypst.NewTypstRenderer()
	out, err := r.RenderImage(context.Background(), goldenSample, "markdown", render.Options{
		Theme:  preset.Theme,
		Width:  preset.WidthPx,
		Height: preset.HeightPx,
		PPI:    preset.PPI,
	})
	if err != nil {
		t.Fatalf("RenderImage realistic-square: %v", err)
	}

	goldenDir := filepath.Join("testdata", "render_image", "golden")
	path := filepath.Join(goldenDir, "realistic-square.png")
	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s (%d bytes)", path, len(out))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("golden mismatch for realistic-square: got %d bytes, want %d", len(out), len(want))
	}
}
