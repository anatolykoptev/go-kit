package html

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/render"
)

func TestCoverPage_Nil(t *testing.T) {
	got := renderCoverPage(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil, got %q", got)
	}
}

func TestCoverPage_AllFields(t *testing.T) {
	cp := &render.CoverPage{
		Title:    "Q1 Report",
		Subtitle: "Security Audit",
		Author:   "Vaelor",
		Date:     "2026-01-15",
		Logo:     "logo.png",
	}
	got := renderCoverPage(cp)
	for _, want := range []string{
		`class="cover-page"`,
		`class="cover-logo" src="logo.png"`,
		`>Q1 Report<`,
		`>Security Audit<`,
		`>Vaelor<`,
		`>2026-01-15<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q; got: %s", want, got)
		}
	}
}

func TestCoverPage_DateDefault(t *testing.T) {
	got := renderCoverPage(&render.CoverPage{Title: "X"})
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(got, today) {
		t.Errorf("expected today's date %s in output; got: %s", today, got)
	}
}

func TestCoverPage_HTMLEscaping(t *testing.T) {
	cp := &render.CoverPage{
		Title:  `<script>alert("x")</script>`,
		Author: `"Evil" & Co.`,
	}
	got := renderCoverPage(cp)
	if strings.Contains(got, "<script>") {
		t.Errorf("raw <script> tag leaked into output: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped &lt;script&gt;; got: %s", got)
	}
	if !strings.Contains(got, "&#34;Evil&#34; &amp; Co.") && !strings.Contains(got, `"Evil" &amp; Co.`) {
		// html.EscapeString encodes " as &#34; and & as &amp;
		t.Errorf("expected author to be escaped; got: %s", got)
	}
}

func TestCoverPage_OnlyLogo(t *testing.T) {
	got := renderCoverPage(&render.CoverPage{Logo: "data:image/png;base64,XXX"})
	if !strings.Contains(got, `src="data:image/png;base64,XXX"`) {
		t.Errorf("expected data URL in src; got: %s", got)
	}
	if strings.Contains(got, `class="cover-title"`) {
		t.Errorf("unexpected title element when only Logo is set")
	}
}

func TestCoverPage_LogoSchemeAllowlist(t *testing.T) {
	// javascript: URL must NOT appear in output.
	got := renderCoverPage(&render.CoverPage{
		Title: "T",
		Logo:  "javascript:alert(1)",
	})
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: URL leaked into output: %s", got)
	}
	if strings.Contains(got, `class="cover-logo"`) {
		t.Errorf("disallowed logo should be skipped entirely; got: %s", got)
	}
	// data: URL IS allowed.
	got = renderCoverPage(&render.CoverPage{
		Logo: "data:image/png;base64,AAAA",
	})
	if !strings.Contains(got, `src="data:image/png;base64,AAAA"`) {
		t.Errorf("data: URL should be rendered; got: %s", got)
	}
	// Relative path IS allowed.
	got = renderCoverPage(&render.CoverPage{Logo: "assets/logo.png"})
	if !strings.Contains(got, `src="assets/logo.png"`) {
		t.Errorf("relative path should be rendered; got: %s", got)
	}
	// vbscript: must NOT appear.
	got = renderCoverPage(&render.CoverPage{Logo: "vbscript:foo"})
	if strings.Contains(got, "vbscript:") {
		t.Errorf("vbscript: URL leaked; got: %s", got)
	}
}

func TestRenderHTML_WithCoverPage(t *testing.T) {
	ctx := context.Background()
	opts := render.Options{CoverPage: &render.CoverPage{Title: "Cover Test"}}
	html, err := RenderHTML(ctx, "# Body\n\nparagraph", opts)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !strings.Contains(html, `class="cover-page"`) {
		t.Errorf("expected cover-page div in output")
	}
	if !strings.Contains(html, `Cover Test`) {
		t.Errorf("expected cover title in output")
	}
	// Ensure order: cover page comes before body
	coverIdx := strings.Index(html, `class="cover-page"`)
	bodyIdx := strings.Index(html, `<h1`)
	if coverIdx == -1 || bodyIdx == -1 || coverIdx > bodyIdx {
		t.Errorf("cover page should precede body; coverIdx=%d bodyIdx=%d", coverIdx, bodyIdx)
	}
}

func TestRenderHTML_NoCoverPageByDefault(t *testing.T) {
	ctx := context.Background()
	html, err := RenderHTML(ctx, "# Body", render.Options{})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if strings.Contains(html, `class="cover-page"`) {
		t.Errorf("cover-page should not appear when Options.CoverPage is nil")
	}
}

// TestCoverPage_LogoFileSchemeBlocked is the SSRF regression guard for the MEDIUM
// finding: file: was in the old allowedLogoSchemes, letting Chrome load arbitrary
// local files (e.g. file:///etc/passwd) and embed them in the rendered PDF.
// This test FAILS on the pre-fix code and PASSES after.
func TestCoverPage_LogoFileSchemeBlocked(t *testing.T) {
	for _, logo := range []string{
		"file:///etc/passwd",
		"file:///proc/self/environ",
		"FILE:///etc/shadow",
	} {
		got := renderCoverPage(&render.CoverPage{Title: "T", Logo: logo})
		if strings.Contains(got, `class="cover-logo"`) {
			t.Errorf("file: logo %q leaked into cover page output:\n%s", logo, got)
		}
	}
}

// TestCoverPage_LogoDataNonImageBlocked guards that bare data: (non-image subtypes)
// are rejected. data:image/* remains allowed (checked in TestCoverPage_OnlyLogo).
func TestCoverPage_LogoDataNonImageBlocked(t *testing.T) {
	for _, logo := range []string{
		"data:text/html,<script>alert(1)</script>",
		"data:application/javascript,alert(1)",
		"data:text/plain,hello",
	} {
		got := renderCoverPage(&render.CoverPage{Title: "T", Logo: logo})
		if strings.Contains(got, `class="cover-logo"`) {
			t.Errorf("non-image data: logo %q leaked into cover page output:\n%s", logo, got)
		}
	}
}
