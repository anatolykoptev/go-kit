package html

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

func TestRenderHTML_Empty(t *testing.T) {
	_, err := RenderHTML(context.Background(), "", render.Options{})
	if err == nil {
		t.Fatal("expected error for empty markdown, got nil")
	}
}

func TestRenderHTML_H1AndTable(t *testing.T) {
	md := "# Title\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"<h1>", "<table>", "<td>1</td>"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected output to contain %q\noutput:\n%s", want, html)
		}
	}
}

func TestRenderHTML_CodeFence(t *testing.T) {
	md := "```go\nfmt.Println(\"hi\")\n```\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With highlighting enabled, chroma wraps the code block: <pre class="chroma"><code>...
	if !strings.Contains(html, "<pre") || !strings.Contains(html, "<code") {
		t.Errorf("expected output to contain <pre ... <code, got:\n%s", html)
	}
}

func TestRenderHTML_TitleExtractionFromFirstH1(t *testing.T) {
	md := "# First\n\n# Second\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<title>First</title>") {
		t.Errorf("expected <title>First</title>, got:\n%s", html)
	}
}

func TestRenderHTML_TitleOverrideWins(t *testing.T) {
	md := "# First\n"
	html, err := RenderHTML(context.Background(), md, render.Options{Title: "Custom"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<title>Custom</title>") {
		t.Errorf("expected <title>Custom</title>, got:\n%s", html)
	}
}

func TestRenderHTML_CustomCSSAppended(t *testing.T) {
	md := "# Heading\n\nsome text\n"
	const marker = ".my-unique-class-xyz { color: magenta; }"
	html, err := RenderHTML(context.Background(), md, render.Options{CustomCSS: marker})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, marker) {
		t.Errorf("expected output to contain custom CSS %q, got:\n%s", marker, html)
	}
	// Ensure custom CSS comes after default CSS (after "body" selector from report.css)
	idxBody := strings.Index(html, "body {")
	idxCustom := strings.Index(html, marker)
	if idxBody == -1 || idxCustom == -1 || idxCustom < idxBody {
		t.Errorf("expected custom CSS after default CSS (body idx=%d, custom idx=%d)", idxBody, idxCustom)
	}
}

func TestRenderHTML_StripsFileURL(t *testing.T) {
	md := "![x](file:///etc/passwd)\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, "file://") {
		t.Errorf("expected file:// to be stripped, got:\n%s", html)
	}
}

func TestRenderHTML_StripsJavaScriptURL(t *testing.T) {
	md := "[click](javascript:alert(1))\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, "javascript:") {
		t.Errorf("expected javascript: to be stripped, got:\n%s", html)
	}
}

func TestRenderHTML_StripsDataURL(t *testing.T) {
	md := "![x](data:text/html,<script>alert(1)</script>)\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, "src=\"data:") || strings.Contains(html, "data:text/html") {
		t.Errorf("expected data: URL to be stripped, got:\n%s", html)
	}
}

func TestRenderHTML_StripsVBScriptURL(t *testing.T) {
	md := "[x](vbscript:msgbox)\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, "href=\"vbscript:") || strings.Contains(html, "vbscript:msgbox") {
		t.Errorf("expected vbscript: URL to be stripped, got:\n%s", html)
	}
}

func TestRenderHTML_CodeBlockHighlighted(t *testing.T) {
	md := "```go\nfunc main() {}\n```\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// chroma class-based formatter emits spans with class="chroma" on wrappers
	// and token-class spans with class names like "kd", "nf", etc. Assert the
	// "chroma" marker exists so a future switch-off of highlighting is caught.
	if !strings.Contains(html, "chroma") {
		t.Errorf("expected highlighted output to contain chroma class markup, got:\n%s", html)
	}
}

func TestRenderHTML_TOC_DisabledByDefault(t *testing.T) {
	md := "# A\n## B\n## C\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, `<nav class="toc">`) {
		t.Errorf("expected no TOC when TOC=false, got:\n%s", html)
	}
}

func TestRenderHTML_TOC_Enabled(t *testing.T) {
	md := "# A\n## B\n## C\n"
	html, err := RenderHTML(context.Background(), md, render.Options{TOC: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`<nav class="toc">`,
		`<h1 id="a">`,
		`<h2 id="b">`,
		`<h2 id="c">`,
		`<a href="#a">A</a>`,
		`<a href="#b">B</a>`,
		`<a href="#c">C</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected output to contain %q\noutput:\n%s", want, html)
		}
	}
}

func TestRenderHTML_TOC_SingleHeadingSkipped(t *testing.T) {
	md := "# Only\n\nsome body text\n"
	html, err := RenderHTML(context.Background(), md, render.Options{TOC: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(html, `<nav class="toc">`) {
		t.Errorf("expected single-heading doc to skip TOC, got:\n%s", html)
	}
}

func TestRenderHTML_TOC_DuplicateHeadingsDisambiguated(t *testing.T) {
	md := "# Foo\n\n# Foo\n"
	html, err := RenderHTML(context.Background(), md, render.Options{TOC: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`id="foo"`,
		`id="foo-2"`,
		`<a href="#foo">Foo</a>`,
		`<a href="#foo-2">Foo</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected output to contain %q\noutput:\n%s", want, html)
		}
	}
}

// TestRenderHTML_DataImagePreserved is the regression guard for the MEDIUM
// finding: the old sanitizeMarkdown regex blanket-replaced ALL data: prefixes
// (including data:image/*) with "#", destroying legitimate inline base64
// images. Goldmark's IsDangerousURL correctly allows data:image/ — this test
// FAILS on the pre-fix code (with sanitizeMarkdown active) and PASSES after.
func TestRenderHTML_DataImagePreserved(t *testing.T) {
	// A minimal valid 1x1 white PNG in base64.
	const dataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	md := "![logo](" + dataURI + ")\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The data URI must survive into the rendered <img src="...">.
	if !strings.Contains(html, "data:image/png") {
		t.Errorf("data:image/png was stripped — goldmark should permit data:image/ URIs\noutput:\n%s", html)
	}
	// Verify the src didn't collapse to "#" (the old sanitizeMarkdown regression).
	if strings.Contains(html, `src="#"`) {
		t.Errorf("data:image/png collapsed to src=\"#\" — sanitizeMarkdown regression\noutput:\n%s", html)
	}
}

func TestRenderHTML_TitleExtractionWithInlineCode(t *testing.T) {
	md := "# Report for `acme-corp`\n"
	html, err := RenderHTML(context.Background(), md, render.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<title>Report for acme-corp</title>") {
		t.Errorf("expected <title>Report for acme-corp</title>, got:\n%s", html)
	}
}
