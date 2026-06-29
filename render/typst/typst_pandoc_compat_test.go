package typst

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

// pngMagicInternal is the 8-byte PNG signature used by internal package tests.
var pngMagicInternal = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func skipIfNoTypstPandoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst binary not on PATH")
	}
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc binary not on PATH")
	}
}

// realisticMarkdownSample is the production trace that exposed the
// pandoc/typst incompatibility on 2026-04-25 (orchestrator agent generating
// a quarterly-report image). It exercises horizontal rules, blockquotes,
// citations, links, lists, fenced code, tables, and footnotes — i.e. every
// realistic document construct that historically triggered "unknown
// variable" errors in stock typst 0.14.
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

// TestSanitizeTypstFromPandoc covers each rewrite rule + interesting edges.
// The negative cases (mid-line `#horizontalrule`, cite with whitespace) are
// load-bearing: they document the explicit choice to leave untouched input
// alone and let typst error loudly when given truly invalid markup, rather
// than silently mangling text that merely happens to mention these tokens.
func TestSanitizeTypstFromPandoc(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "horizontalrule alone",
			in:   "#horizontalrule",
			want: "#line(length: 100%)",
		},
		{
			name: "horizontalrule with trailing whitespace",
			in:   "#horizontalrule   \t",
			want: "#line(length: 100%)",
		},
		{
			name: "horizontalrule between paragraphs",
			in:   "para a\n\n#horizontalrule\n\npara b",
			want: "para a\n\n#line(length: 100%)\n\npara b",
		},
		{
			name: "horizontalrule mid-line is unchanged",
			in:   "not really a #horizontalrule call",
			want: "not really a #horizontalrule call",
		},
		{
			name: "blockquote single line",
			in:   "#blockquote[content]",
			want: "#quote(block: true)[content]",
		},
		{
			name: "blockquote multi-line",
			in:   "#blockquote[\nfirst\nsecond\n]",
			want: "#quote(block: true)[\nfirst\nsecond\n]",
		},
		{
			name: "bare word #blockquote without bracket is unchanged",
			in:   "we mention #blockquote in prose",
			want: "we mention #blockquote in prose",
		},
		{
			// Without a #bibliography directive the label form would still
			// error, so rule 4 downgrades the cite to a plaintext marker.
			name: "cite simple identifier (no bibliography → plaintext)",
			in:   `#cite("foo")`,
			want: `[\@foo]`,
		},
		{
			name: "cite alphanumeric (no bibliography → plaintext)",
			in:   `#cite("foo2026")`,
			want: `[\@foo2026]`,
		},
		{
			name: "cite with dash and dot (no bibliography → plaintext)",
			in:   `#cite("with-dash.dot")`,
			want: `[\@with-dash.dot]`,
		},
		{
			// Bibliography present → label form is correct, rule 4 no-ops.
			name: "cite simple identifier with bibliography keeps label form",
			in:   "#bibliography(\"refs.bib\")\n\n" + `Cite #cite("foo") here.`,
			want: "#bibliography(\"refs.bib\")\n\n" + "Cite #cite(<foo>) here.",
		},
		{
			name: "cite with whitespace is unchanged",
			in:   `#cite("has space")`,
			want: `#cite("has space")`,
		},
		{
			name: "all three rewrites composed",
			in: "intro\n\n#horizontalrule\n\n#blockquote[wisdom]\n\n" +
				`See #cite("foo2026") here.`,
			want: "intro\n\n#line(length: 100%)\n\n#quote(block: true)[wisdom]\n\n" +
				`See [\@foo2026] here.`,
		},
		{
			name: "cite label form falls back without bibliography",
			in:   `As shown in #cite(<koptev2026>), the market grows.`,
			want: `As shown in [\@koptev2026], the market grows.`,
		},
		{
			name: "cite label form preserved when bibliography is present",
			in:   "#bibliography(\"refs.bib\")\n\nSee #cite(<koptev2026>).",
			want: "#bibliography(\"refs.bib\")\n\nSee #cite(<koptev2026>).",
		},
		{
			name: "remote image https → marker",
			in:   `#image("https://example.com/img.png")`,
			want: `[image]`,
		},
		{
			name: "remote image http → marker",
			in:   `#image("http://example.com/img.png")`,
			want: `[image]`,
		},
		{
			name: "remote image inside figure preserves caption",
			in:   "#figure([#image(\"https://example.com/img.png\")],\n  caption: [diagram alt])",
			want: "#figure([[image]],\n  caption: [diagram alt])",
		},
		{
			name: "local image path is unchanged",
			in:   `#image("/tmp/local.png")`,
			want: `#image("/tmp/local.png")`,
		},
		{
			name: "relative image path is unchanged",
			in:   `#image("./assets/logo.png")`,
			want: `#image("./assets/logo.png")`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTypstFromPandoc(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizeTypstFromPandoc mismatch\nin:   %q\ngot:  %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSanitizeTypstFromPandoc_NoOpOnTrivialInput locks in that trivial markdown
// (the shape the existing 3 goldens were built from) is byte-equal after
// sanitization — guaranteeing the existing goldens cannot drift due to this
// patch.
func TestSanitizeTypstFromPandoc_NoOpOnTrivialInput(t *testing.T) {
	in := "= Test card\n<test-card>\n\nA simple example.\n"
	if got := sanitizeTypstFromPandoc(in); got != in {
		t.Fatalf("sanitizer mutated trivial input\nin:  %q\ngot: %q", in, got)
	}
}

// TestCompileTypst_HorizontalRuleIsRejected is the regression-defensive
// negative test: it verifies that stock typst 0.14 STILL rejects
// `#horizontalrule` markup. If a future typst release accepts it natively,
// this test fails and the sanitizer rule (and the test rows above) can be
// retired. Without this contract test, a silent deletion of the sanitizer
// would re-open the production bug undetected.
func TestCompileTypst_HorizontalRuleIsRejected(t *testing.T) {
	skipIfNoTypstPandoc(t)
	raw := "= Title\n\n#horizontalrule\n\nBody.\n"
	_, err := compileTypst(context.Background(), raw, typstOutput{Format: typstFormatPDF})
	if err == nil {
		t.Fatal("expected typst to reject #horizontalrule, got nil error")
	}
	if !strings.Contains(err.Error(), "unknown variable: horizontalrule") {
		t.Fatalf("wrong error message — sanitizer contract broken? got: %v", err)
	}
}

// TestTypstRenderImage_RealisticContent is the end-to-end integration test
// that pre-sanitizer would have failed with `unknown variable: horizontalrule`.
// It renders the realistic markdown sample through RenderImage and asserts
// PNG bytes are produced.
func TestTypstRenderImage_RealisticContent(t *testing.T) {
	skipIfNoTypstPandoc(t)
	r := NewTypstRenderer()
	out, err := r.RenderImage(context.Background(), realisticMarkdownSample, "markdown", render.Options{
		Width:  800,
		Height: 1200,
		PPI:    96,
		Theme:  "report",
	})
	if err != nil {
		t.Fatalf("RenderImage realistic sample: %v", err)
	}
	if len(out) < 8 || !bytes.Equal(out[:8], pngMagicInternal) {
		t.Fatalf("output is not PNG (first 8 bytes: % x)", out[:min(8, len(out))])
	}
}

// TestTypstRenderImage_RemoteImage is the regression test for Rule 5.
// Pre-Rule-5, markdown with an external image URL would fail with
// "file not found" because typst tries to read http://... as a local path.
func TestTypstRenderImage_RemoteImage(t *testing.T) {
	skipIfNoTypstPandoc(t)
	const md = "# Title\n\n" +
		"Paragraph before.\n\n" +
		"![Diagram of architecture](https://example.com/diagram.png)\n\n" +
		"Paragraph after.\n"
	r := NewTypstRenderer()
	out, err := r.RenderImage(context.Background(), md, "markdown", render.Options{
		Width: 800, Height: 600, PPI: 96, Theme: "report",
	})
	if err != nil {
		t.Fatalf("RenderImage with remote image: %v", err)
	}
	if len(out) < 8 || !bytes.Equal(out[:8], pngMagicInternal) {
		t.Fatalf("output is not PNG (first 8 bytes: % x)", out[:min(8, len(out))])
	}
}
// NOTE: TestTypstRenderImage_GoldenRealisticContent lives in typst_image_test.go
// (package typst_test) because it uses the updateGolden flag defined there.

// ── resume theme tests ────────────────────────────────────────────────────────

// resumeMarkdownSample is a content-rich one-page CV that exercises all
// typical resume constructs: multiple ## sections, bullet lists, a table,
// inline code, and enough text that the "report" theme would spill to 2 pages.
const resumeMarkdownSample = "# Jane Smith\n\n" +
	"jane@example.com | github.com/janesmith | San Francisco, CA\n\n" +
	"## Summary\n\n" +
	"Staff engineer with 10 years of experience building distributed systems " +
	"at scale. Expert in Go, Rust, and TypeScript. Strong track record of " +
	"delivering reliable, high-throughput data pipelines and developer tooling.\n\n" +
	"## Experience\n\n" +
	"### Senior Staff Engineer — Acme Corp (2020–present)\n\n" +
	"- Designed a multi-region event pipeline handling 2M events/s, reducing p99 by 60%\n" +
	"- Led a team of 8 engineers across 3 time zones to deliver a new query engine\n" +
	"- Reduced infrastructure cost by $2M/yr via shared-target Cargo build cache\n\n" +
	"### Staff Engineer — Beta Inc (2017–2020)\n\n" +
	"- Rewrote the authentication service in Go; cut latency from 120ms to 8ms\n" +
	"- Shipped zero-downtime migration of 50M rows across 12 sharded Postgres clusters\n" +
	"- Mentored 6 mid-level engineers; 4 promoted to senior within 18 months\n\n" +
	"### Senior Engineer — Gamma LLC (2014–2017)\n\n" +
	"- Built the company's first ML serving layer (ONNX, gRPC, 99.99% uptime)\n" +
	"- Integrated SFU-based real-time video into a B2B SaaS product (WebRTC, Rust)\n\n" +
	"## Skills\n\n" +
	"| Category | Technologies |\n" +
	"|----------|--------------|\n" +
	"| Languages | Go, Rust, TypeScript, Python, SQL |\n" +
	"| Infra | Docker, Postgres, Redis, Kubernetes |\n" +
	"| Observability | Prometheus, Grafana, Jaeger, Loki |\n\n" +
	"## Education\n\n" +
	"**B.S. Computer Science** — MIT, 2014. GPA 3.9. Thesis: _Adaptive congestion control in WebRTC media planes_.\n"

// TestResolveTypstTheme_Resume verifies that "resume" resolves to the resume
// theme and NOT the report fallback.
func TestResolveTypstTheme_Resume(t *testing.T) {
	got := resolveTypstTheme("resume")
	report := resolveTypstTheme("report")
	if got == report {
		t.Fatal(`resolveTypstTheme("resume") returned the report theme — new case not wired`)
	}
}

// TestResumeThemePreamble_TightValues guards that the key tightness knobs
// cannot silently regress to the looser "report" values, and that the
// US-resume-specific design decisions (Letter paper, left-align, no footer)
// cannot be accidentally reverted.
func TestResumeThemePreamble_TightValues(t *testing.T) {
	p := typstThemeResume.preamble

	// Values that MUST be present.
	must := []struct {
		label   string
		contain string
	}{
		{"us-letter paper", `paper:  "us-letter"`},
		{"x-margin 20mm", "margin: (x: 20mm"},
		{"tight top margin", "top: 14mm"},
		{"tight bottom margin", "bottom: 14mm"},
		{"text size 10.5pt", "size: 10.5pt"},
		{"tight par leading", "leading: 0.6em"},
		{"tight par spacing", "spacing: 0.7em"},
		{"h1 small vspace", "v(3mm, weak: true)"},
		{"h1 size 16pt", "size: 16pt"},
		{"h2 small vspace", "v(2.5mm, weak: true)"},
		{"h2 size 12pt", "size: 12pt"},
	}
	for _, c := range must {
		if !strings.Contains(p, c.contain) {
			t.Errorf("resume preamble missing %s: want substring %q", c.label, c.contain)
		}
	}

	// Values that MUST NOT be present (design regressions).
	mustNot := []struct {
		label  string
		banned string
	}{
		{"no justify: true (rivers)", "justify: true"},
		{"no footer date block", "footer:"},
		{"no a4 paper", `"a4"`},
	}
	for _, c := range mustNot {
		if strings.Contains(p, c.banned) {
			t.Errorf("resume preamble contains banned %s: found substring %q", c.label, c.banned)
		}
	}
}

// TestResumeTheme_ReportUnchanged ensures that modifying the resume theme
// never accidentally mutates the report theme.
func TestResumeTheme_ReportUnchanged(t *testing.T) {
	const wantReportMargin = "margin: (x: 24mm"
	if !strings.Contains(typstThemeReport.preamble, wantReportMargin) {
		t.Errorf("report theme margin changed — expected %q", wantReportMargin)
	}
}

// pdfMagic is the 5-byte PDF signature (%PDF-).
var pdfMagic = []byte{'%', 'P', 'D', 'F', '-'}

// TestTypstRender_ResumePDF is the end-to-end integration test: it renders
// the multi-section resume markdown with the "resume" theme and asserts a
// valid PDF is produced. Skips if typst+pandoc are not installed.
func TestTypstRender_ResumePDF(t *testing.T) {
	skipIfNoTypstPandoc(t)
	r := NewTypstRenderer()
	out, err := r.Render(context.Background(), resumeMarkdownSample, "markdown", render.Options{
		Theme: "resume",
		Title: "Jane Smith — Resume",
	})
	if err != nil {
		t.Fatalf("Render resume: %v", err)
	}
	if len(out) < 5 || !bytes.Equal(out[:5], pdfMagic) {
		t.Fatalf("output is not PDF (first 5 bytes: % x)", out[:min(5, len(out))])
	}
	t.Logf("resume PDF: %d bytes", len(out))
}
