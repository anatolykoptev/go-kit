package typst

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// Tests for the second typst injection vector — opts.Title is interpolated
// into the .typ source at two sites, and typst's # opens code mode wherever
// it lands, so an unescaped title like `#import "@preview/cetz:0.3.1"` is
// executable typst. This fully bypasses the RawTypstPassthrough body guard
// because the title never goes through pandoc.
//
// The fix renders the title as a typst string literal (#"…") with \ and "
// escaped — a small, complete, auditable escape grammar. Trying to enumerate
// and escape typst's markup metacharacters instead would be a missed-character
// bypass waiting to happen.
//
// Two tests, one per site, each asserting on the OBSERVABLE assembled .typ
// source (captured via an injected compile), never on an argv slice. Each
// also asserts an ordinary title still renders — an escape that mangles
// normal text is a regression, and a test that only checks the attack is
// half a gate.
//
// Verified against pandoc + typst on the host (2026-08-22). pandoc and typst
// are on PATH, so these must actually run — a skipped test is a false green.

// titleInjectionPayload is a typst directive that, if it reaches the .typ
// source as bare executable typst, downloads and executes a remote package.
// No newline or trick is needed — # opens code mode wherever the title lands.
const titleInjectionPayload = `#import "@preview/cetz:0.3.1"`

// TestTitleEscape_TitleBlockIsInert verifies the concatenated title heading
// (site 1: "= " + opts.Title in buildTypstSource) renders the title as a
// typst string literal, not as bare executable typst. Uses "minimal" — a
// theme whose preamble does NOT emit {{.Title}} — so only the heading can
// carry the payload. If the preamble site were also unescaped, it would not
// matter here because minimal's preamble has no {{.Title}} to splice into.
//
// Mutation that would RED this test (still compiles):
//
//	render/typst/typst.go — in buildTypstSource, revert the titleBlock
//	assignment from
//	  titleBlock = "= " + typstStringLiteral(opts.Title) + "\n\n"
//	to
//	  titleBlock = "= " + opts.Title + "\n\n"
//
// The payload would appear as `= #import "..."` (executable — # opens code
// mode after =) and the assertion would fail.
func TestTitleEscape_TitleBlockIsInert(t *testing.T) {
	skipIfNoPandoc(t)
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), "Body.\n", "markdown", render.Options{
		Theme: "minimal", // preamble has no {{.Title}} — isolates the title block
		Title: titleInjectionPayload,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The payload must NOT appear as bare executable typst after "= ".
	// "= #import" means the # opened code mode and the import executes.
	if strings.Contains(*src, "= #import") {
		t.Fatalf("title payload reached .typ source as executable typst (unescaped title block):\n%s", head(*src))
	}
	// The escaped string-literal form must be present.
	if !strings.Contains(*src, `= #"`) {
		t.Fatalf("title block did not render as a typst string literal:\n%s", head(*src))
	}
}

// TestTitleEscape_PreambleIsInert verifies the preamble template (site 2:
// {{.Title}} in themes.go, executed via text/template with
// typstDocData{Title: ...}) renders the title as a typst string literal.
// Uses "report" — a theme that DOES emit {{.Title}} in its running header
// — so the payload can only be neutralized by the preamble-side escape.
//
// Mutation that would RED this test (still compiles):
//
//	render/typst/typst.go — in buildTypstSource, revert the typstDocData
//	assignment from
//	  typstDocData{Title: typstStringLiteral(opts.Title)}
//	to
//	  typstDocData{Title: opts.Title}
//
// The payload would appear as [#import "..."] (executable inside content
// brackets — # opens code mode inside []) and the assertion would fail.
func TestTitleEscape_PreambleIsInert(t *testing.T) {
	skipIfNoPandoc(t)
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), "Body.\n", "markdown", render.Options{
		Theme: "report", // preamble emits {{.Title}} in the header — isolates the preamble
		Title: titleInjectionPayload,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// The payload must NOT appear as bare executable typst inside content
	// brackets. "[#import" means the # opened code mode inside [] and the
	// import executes.
	if strings.Contains(*src, "[#import") {
		t.Fatalf("title payload reached preamble as executable typst (unescaped {{.Title}}):\n%s", head(*src))
	}
	// The escaped string-literal form must be present in the preamble.
	if !strings.Contains(*src, `[#"`) {
		t.Fatalf("preamble did not render the title as a typst string literal:\n%s", head(*src))
	}
}

// TestTitleEscape_OrdinaryTitleRenders verifies that a normal title without
// typst metacharacters still renders readably after escaping — an escape
// that mangles normal text is a regression. Checks both sites: the title
// block heading and the report preamble header.
func TestTitleEscape_OrdinaryTitleRenders(t *testing.T) {
	skipIfNoPandoc(t)
	const ordinary = "Quarterly Review"
	r, src := capturePassthroughSource(t)
	if _, err := r.Render(context.Background(), "Body.\n", "markdown", render.Options{
		Theme: "report",
		Title: ordinary,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Title block heading: `= #"Quarterly Review"`
	if !strings.Contains(*src, `= #"`+ordinary+`"`) {
		t.Errorf("ordinary title not rendered in title block:\n%s", head(*src))
	}
	// Preamble header: `[#"Quarterly Review"]`
	if !strings.Contains(*src, `[#"`+ordinary+`"]`) {
		t.Errorf("ordinary title not rendered in preamble header:\n%s", head(*src))
	}
}
