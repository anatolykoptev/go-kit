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
// typst. pandoc is needed, hence skipIfNoPandoc; typst is not, PROVIDED the
// wiring is correct. A mutant that bypasses the injected step shells out to the
// real binary and reds with a typst error about page-number templates — still
// RED, but pointing at a bogus cause, so the stub records that it was reached.
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

	capture := func(t *testing.T) (*TypstRenderer, *string, *bool) {
		t.Helper()
		var got string
		var reached bool
		r := NewTypstRenderer()
		r.compile = func(_ context.Context, source string, _ typstOutput) ([]byte, error) {
			reached = true
			got = source
			// Bytes are irrelevant here; only the source is under test. PNG
			// magic keeps any future caller-side sniffing honest.
			return []byte{0x89, 'P', 'N', 'G'}, nil
		}
		return r, &got, &reached
	}

	opts := render.Options{Title: title, Theme: themeCard, TOC: true, Width: 600, Height: 400, PPI: 144}

	rPDF, pdfSrc, pdfReached := capture(t)
	if _, err := rPDF.Render(context.Background(), body, "markdown", opts); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rIMG, imgSrc, imgReached := capture(t)
	if _, err := rIMG.RenderImage(context.Background(), body, "markdown", opts); err != nil {
		// The bypass check has to live in the ERROR branch, not after it. A
		// bypassed PDF compile succeeds (PDFs are multi-page), so Render reaches
		// the guard below with err == nil; a bypassed PNG compile shells out to
		// the real typst and fails first with a page-number-template error —
		// RED, but naming a cause that is not the defect.
		if !*imgReached {
			t.Fatalf("RenderImage never called the injected compile step — it bypassed r.compiler() (underlying: %v)", err)
		}
		t.Fatalf("RenderImage: %v", err)
	}

	if !*pdfReached {
		t.Fatal("Render never called the injected compile step — it bypassed r.compiler(); every assertion below would be about an empty string")
	}
	if !*imgReached {
		t.Fatal("RenderImage never called the injected compile step — it bypassed r.compiler()")
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
