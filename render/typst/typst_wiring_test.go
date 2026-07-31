package typst

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// TestEntryPointSourceWiring guards which assembler each public entry point
// uses. pdfSource and imageSource have identical signatures, so swapping them
// compiles; the earlier round split them out precisely so their decisions were
// testable, then left the CALL unguarded, which is the same gap one level up.
//
// Observes the composed source through the injected compile step rather than
// re-deriving it, so the assertion is about what Render actually handed to
// typst. No typst binary needed; pandoc is, hence skipIfNoPandoc.
//
// The discriminators are the two decisions that observably differ:
//   - title block: Render always injects, RenderImage obeys the theme's
//     OmitsTitleBlockOnImage (card declares it)
//   - geometry: only imageSource emits the page-size override
//
// opts.TOC is deliberately NOT used as a discriminator. Measured on pandoc
// 3.1.3: --toc without --standalone produces byte-identical output to no --toc
// at all (same md5), and this package never passes --standalone. So TOC is
// inert here and imageSource forcing it off guards nothing. Tracked separately;
// a test built on it would assert a difference that does not exist.
func TestEntryPointSourceWiring(t *testing.T) {
	skipIfNoPandoc(t)

	const title = "Quarterly Review"
	const body = "# Body Heading\n\nBody text.\n\n## Second\n\nMore.\n"

	capture := func(t *testing.T) (*TypstRenderer, *string) {
		t.Helper()
		var got string
		r := NewTypstRenderer()
		r.compile = func(_ context.Context, source string, _ typstOutput) ([]byte, error) {
			got = source
			// Bytes are irrelevant here; only the source is under test. PNG
			// magic keeps any future caller-side sniffing honest.
			return []byte{0x89, 'P', 'N', 'G'}, nil
		}
		return r, &got
	}

	opts := render.Options{Title: title, Theme: themeCard, TOC: true, Width: 600, Height: 400, PPI: 144}

	rPDF, pdfSrc := capture(t)
	if _, err := rPDF.Render(context.Background(), body, "markdown", opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rIMG, imgSrc := capture(t)
	if _, err := rIMG.RenderImage(context.Background(), body, "markdown", opts); err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	if !strings.Contains(*pdfSrc, "= "+title) {
		t.Errorf("Render did not inject the title block — it is not assembling through pdfSource:\n%s", head(*pdfSrc))
	}
	if strings.Contains(*pdfSrc, "#set page(width:") {
		t.Errorf("Render emitted a geometry override — it is assembling through imageSource:\n%s", head(*pdfSrc))
	}

	if strings.Contains(*imgSrc, "= "+title) {
		t.Errorf("RenderImage injected the title block on a theme declaring OmitsTitleBlockOnImage — it is assembling through pdfSource:\n%s", head(*imgSrc))
	}
	if !strings.Contains(*imgSrc, "#set page(width:") {
		t.Errorf("RenderImage omitted the geometry override — it is assembling through pdfSource:\n%s", head(*imgSrc))
	}

	// The paths must actually differ. Without this the assertions above could
	// all hold on two identical sources if a future refactor collapsed them.
	if *pdfSrc == *imgSrc {
		t.Error("Render and RenderImage produced byte-identical sources — the two paths have collapsed into one")
	}
}

// TestZeroValueRendererCompiles pins that a TypstRenderer built as a literal,
// which the exported type permits, still resolves a compile step instead of
// panicking on a nil field.
func TestZeroValueRendererCompiles(t *testing.T) {
	var r TypstRenderer
	if r.compiler() == nil {
		t.Error("zero-value TypstRenderer has no compile step — a literal-constructed renderer would panic")
	}
}

func head(s string) string {
	const n = 400
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
