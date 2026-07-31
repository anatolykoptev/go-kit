package typst

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// customPreamble is deliberately not valid styling — it only needs to be
// identifiable in the assembled source. The marker cannot occur in any built-in
// theme, so its presence proves the caller's string reached the document and the
// absence of the theme's own page rule proves it replaced rather than appended.
const customPreamble = `#let CALLER_SUPPLIED_PREAMBLE = true
#set page(paper: "us-legal")
`

// A caller-supplied preamble must replace the named theme's, not merge with it.
// Silent failure mode this guards: TypstPreamble is accepted and ignored, the
// document renders in the built-in theme, and the output still looks like a
// plausible resume — so nothing errors and the wrong house style ships.
func TestBuildTypstSource_CustomPreambleReplacesTheme(t *testing.T) {
	skipIfNoTypstPandoc(t)

	r := NewTypstRenderer()
	src, err := r.buildTypstSource(
		context.Background(), "# Heading\n\nBody text.\n", "markdown",
		render.Options{Theme: "report", TypstPreamble: customPreamble}, "", false,
	)
	if err != nil {
		t.Fatalf("buildTypstSource: %v", err)
	}
	if !strings.Contains(src, "CALLER_SUPPLIED_PREAMBLE") {
		t.Errorf("caller preamble missing from assembled source:\n%s", src)
	}
	if strings.Contains(src, `paper:  "a4"`) {
		t.Errorf("report theme preamble still present; custom preamble appended instead of replacing:\n%s", src)
	}
	if !strings.Contains(src, "Body text.") {
		t.Errorf("converted body missing from assembled source:\n%s", src)
	}
}

// An empty TypstPreamble must leave existing callers on the theme path.
func TestBuildTypstSource_EmptyPreambleUsesTheme(t *testing.T) {
	skipIfNoTypstPandoc(t)

	r := NewTypstRenderer()
	src, err := r.buildTypstSource(
		context.Background(), "Body text.\n", "markdown",
		render.Options{Theme: "resume"}, "", false,
	)
	if err != nil {
		t.Fatalf("buildTypstSource: %v", err)
	}
	if !strings.Contains(src, `paper:  "us-letter"`) {
		t.Errorf("resume theme preamble not used when TypstPreamble empty:\n%s", src)
	}
	if strings.Contains(src, "CALLER_SUPPLIED_PREAMBLE") {
		t.Error("caller preamble leaked into a theme-only render")
	}
}

// A custom preamble goes through the same template pass as a built-in one, so
// {{.Title}} resolves for callers that reuse the placeholder.
func TestBuildTypstSource_CustomPreambleHonorsTitleTemplate(t *testing.T) {
	skipIfNoTypstPandoc(t)

	r := NewTypstRenderer()
	src, err := r.buildTypstSource(
		context.Background(), "Body text.\n", "markdown",
		render.Options{
			Title:         "Quarterly Review",
			TypstPreamble: "#let docTitle = \"{{.Title}}\"\n",
		}, "", true,
	)
	if err != nil {
		t.Fatalf("buildTypstSource: %v", err)
	}
	if !strings.Contains(src, `#let docTitle = "Quarterly Review"`) {
		t.Errorf("{{.Title}} not substituted in caller preamble:\n%s", src)
	}
}
