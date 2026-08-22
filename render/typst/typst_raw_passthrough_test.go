package typst

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// Tests for the RawTypstPassthrough option — the pandoc raw-attribute hole
// that lets {=typst} blocks pass through as executable typst. The option
// defaults to OFF (secure); callers with operator-controlled input opt in.
//
// Every test asserts on the OBSERVABLE typst source that reaches the compile
// step (captured via an injected compile), never on the argv slice alone —
// pandoc silently ignores a malformed extension name, which would pass an
// argv assertion while leaving the hole wide open.
//
// Verified against pandoc 3.1.3 + typst 0.14.2 on the host (2026-08-22).
// The shipping container may differ; see the report for version notes.

// rawTypstMarker is a unique directive that only appears in the typst source
// if a {=typst} raw block passed through pandoc unescaped. The 999pt size
// ensures it never collides with a theme preamble's own #set text() call.
const rawTypstMarker = "#set text(size: 999pt)"

// markdownWithRawTypst is markdown containing a {=typst} raw block. When
// pandoc's raw_attribute extension is enabled, the block passes through as
// bare executable typst. When disabled (markdown-raw_attribute), pandoc
// escapes it into an inert code span: `{=typst} #set text(size: 999pt)`.
const markdownWithRawTypst = "# Title\n\nText.\n\n" +
	"```{=typst}\n" + rawTypstMarker + "\n```\n\nMore.\n"

// htmlWithTypstCodeBlock is html containing a <pre><code class="typst">
// block. Pandoc's html reader has no raw_attribute extension (confirmed with
// pandoc 3.1.3: "The extension raw_attribute is not supported for html"),
// so this produces a fenced typst code block (```typst … ```) — displayed
// as code by typst, NOT executed — regardless of the RawTypstPassthrough
// option. There is no passthrough hole to close for html.
const htmlWithTypstCodeBlock = "<h1>Title</h1>\n<p>Text.</p>\n" +
	"<pre><code class=\"typst\">" + rawTypstMarker + "</code></pre>\n"

// capturePassthroughSource returns a TypstRenderer whose compile step
// captures the assembled typst source instead of running typst. This lets
// a test assert on what actually reaches typst — the observable pandoc
// output — rather than on the argv slice.
func capturePassthroughSource(t *testing.T) (*TypstRenderer, *string) {
	t.Helper()
	var got string
	r := NewTypstRenderer()
	r.compile = func(_ context.Context, source string, _ typstOutput) ([]byte, error) {
		got = source
		return []byte("%PDF-"), nil
	}
	return r, &got
}

// TestRawTypstPassthrough_DefaultIsSecure verifies that with no option set
// (the zero-value default), a {=typst} raw block in markdown does NOT reach
// the typst source as executable typst. Pandoc's raw_attribute extension is
// disabled, so the block is escaped into an inert code span.
//
// Mutation that would RED this test (still compiles):
//	render/typst/typst.go — in pandocConvert, change the fromFmt assignment
//	from "markdown-raw_attribute" back to "markdown" when the option is off.
//	The raw block would pass through as bare #set text(size: 999pt) and the
//	assertion would fail.
func TestRawTypstPassthrough_DefaultIsSecure(t *testing.T) {
	skipIfNoPandoc(t)
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), markdownWithRawTypst, "markdown", render.Options{
		Theme: "report",
		// RawTypstPassthrough not set — zero-value default is OFF (secure).
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// When blocked, pandoc escapes the raw block into a code span:
	//   `{=typst} #set text(size: 999pt)`
	escaped := "`{=typst} " + rawTypstMarker + "`"
	if strings.Contains(*src, escaped) {
		return // good — escaped, not executable
	}
	if strings.Contains(*src, rawTypstMarker) {
		t.Fatalf("raw typst directive passed through as executable by default (option is OFF):\n%s", head(*src))
	}
	t.Fatalf("marker disappeared from output — test input may be wrong:\n%s", head(*src))
}

// TestRawTypstPassthrough_OptInWorks verifies that with the option enabled,
// a {=typst} raw block in markdown DOES pass through to the typst source
// verbatim as executable typst. This is what go-job's ligaPreamble and
// assembleResumeHeader rely on.
//
// Mutation that would RED this test (still compiles):
//	render/typst/typst.go — in pandocConvert, always use
//	"markdown-raw_attribute" regardless of the option. The raw block would
//	be escaped even when the caller opted in, and the assertion would fail.
func TestRawTypstPassthrough_OptInWorks(t *testing.T) {
	skipIfNoPandoc(t)
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), markdownWithRawTypst, "markdown", render.Options{
		Theme:               "report",
		RawTypstPassthrough: true,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// When enabled, the raw block passes through as bare executable typst.
	if !strings.Contains(*src, rawTypstMarker) {
		t.Fatalf("raw typst directive did NOT pass through with option enabled:\n%s", head(*src))
	}
	escaped := "`{=typst} " + rawTypstMarker + "`"
	if strings.Contains(*src, escaped) {
		t.Fatalf("raw typst directive was escaped despite option being enabled — passthrough is broken:\n%s", head(*src))
	}
}

// TestRawTypstPassthrough_HTMLPathIsSecure verifies that the html input path
// does not have the raw passthrough hole. Pandoc's html reader has no
// raw_attribute extension (confirmed with pandoc 3.1.3), and html
// <pre><code> blocks produce fenced typst code blocks (```typst … ```)
// which typst displays as code, NOT executes.
//
// This is the "html path" test required by the task. The html path does NOT
// have the same hole as markdown, so there is no extension to disable; the
// test confirms the path is inherently secure by checking the marker does
// not appear as a bare executable line.
//
// Mutation that would RED this test (still compiles):
//	render/typst/typst.go — if a future change made pandocConvert pass
//	"-f html+raw_attribute" (which pandoc 3.1.3 rejects with exit 23), the
//	Render call would fail and the test would RED on the Render error. If
//	pandoc added html raw_attribute support in a future version and the
//	code enabled it, the marker would appear as a bare line and the
//	assertion would fail.
func TestRawTypstPassthrough_HTMLPathIsSecure(t *testing.T) {
	skipIfNoPandoc(t)
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), htmlWithTypstCodeBlock, "html", render.Options{
		Theme: "report",
		// RawTypstPassthrough not set — but html has no hole regardless.
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The html path produces a fenced code block (```typst … ```), which
	// typst displays as code, NOT executes.
	if !strings.Contains(*src, "```typst") {
		t.Fatalf("html code block did not produce a fenced typst block — output shape changed:\n%s", head(*src))
	}
	// The marker must NOT appear as a bare executable line OUTSIDE a fenced
	// code block. Inside a fence it's inert (displayed, not executed); the
	// hole would be the marker appearing as a bare line that typst executes.
	inFence := false
	for _, line := range strings.Split(*src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence && trimmed == rawTypstMarker {
			t.Fatalf("html path emitted bare executable typst outside a fenced block — the hole exists for html too:\n%s", head(*src))
		}
	}
}
