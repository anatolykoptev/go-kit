package html

import (
	"context"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/render"
)

const mermaidFence = "```mermaid\ngraph TD; A-->B\n```\n"

func TestMermaid_Disabled(t *testing.T) {
	ctx := context.Background()
	out, err := RenderHTML(ctx, "# Title\n\n"+mermaidFence, render.Options{})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if strings.Contains(out, `class="mermaid"`) {
		t.Fatalf("expected no mermaid class when disabled; got:\n%s", out)
	}
	if !strings.Contains(out, "<pre") {
		t.Fatalf("expected normal fenced code markup (<pre> element); got:\n%s", out)
	}
}

func TestMermaid_Enabled(t *testing.T) {
	ctx := context.Background()
	out, err := RenderHTML(ctx, "# Title\n\n"+mermaidFence, render.Options{Mermaid: true})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !strings.Contains(out, `<pre class="mermaid">`) {
		t.Fatalf("expected <pre class=\"mermaid\"> in output; got:\n%s", out)
	}
	if !strings.Contains(out, "graph TD; A--&gt;B") {
		t.Fatalf("expected escaped diagram source in output; got:\n%s", out)
	}
}

func TestMermaid_HeadScript(t *testing.T) {
	s := mermaidHeadScript()
	if s == "" {
		t.Fatal("mermaidHeadScript() returned empty string")
	}
	if !strings.Contains(s, "cdn.jsdelivr.net") {
		t.Fatalf("head script missing CDN URL; got: %s", s)
	}
	if !strings.Contains(s, "mermaid.initialize") {
		t.Fatalf("head script missing initializer; got: %s", s)
	}
	// SRI hardening: require a specific version pin (not a floating @11)
	// and a sha384 integrity attribute with crossorigin.
	if !strings.Contains(s, "mermaid@"+mermaidVersion+"/") {
		t.Fatalf("head script missing pinned version %q; got: %s", mermaidVersion, s)
	}
	if !strings.Contains(s, `integrity="sha384-`) {
		t.Fatalf("head script missing SRI integrity attribute; got: %s", s)
	}
	if !strings.Contains(s, `crossorigin="anonymous"`) {
		t.Fatalf("head script missing crossorigin=anonymous; got: %s", s)
	}
}

func TestMermaid_HeadScriptInDocument(t *testing.T) {
	ctx := context.Background()
	md := "# T\n\n" + mermaidFence

	enabled, err := RenderHTML(ctx, md, render.Options{Mermaid: true})
	if err != nil {
		t.Fatalf("RenderHTML enabled: %v", err)
	}
	if !strings.Contains(enabled, "cdn.jsdelivr.net") {
		t.Fatalf("expected head script in document when mermaid enabled; got:\n%s", enabled)
	}
	if !strings.Contains(enabled, "mermaid.initialize") {
		t.Fatalf("expected mermaid.initialize in document when enabled; got:\n%s", enabled)
	}

	disabled, err := RenderHTML(ctx, md, render.Options{})
	if err != nil {
		t.Fatalf("RenderHTML disabled: %v", err)
	}
	if strings.Contains(disabled, "cdn.jsdelivr.net") {
		t.Fatalf("did not expect head script when mermaid disabled; got:\n%s", disabled)
	}
	if strings.Contains(disabled, "mermaid.initialize") {
		t.Fatalf("did not expect mermaid.initialize when disabled; got:\n%s", disabled)
	}
}

func TestMermaid_MultipleBlocks(t *testing.T) {
	ctx := context.Background()
	md := "# T\n\n" +
		"```mermaid\ngraph TD; A-->B\n```\n\n" +
		"Some text.\n\n" +
		"```mermaid\nsequenceDiagram; X->>Y: hi\n```\n"
	out, err := RenderHTML(ctx, md, render.Options{Mermaid: true})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	count := strings.Count(out, `<pre class="mermaid">`)
	if count != 2 {
		t.Fatalf("expected 2 mermaid blocks, got %d; output:\n%s", count, out)
	}
	if !strings.Contains(out, "graph TD; A--&gt;B") {
		t.Fatalf("missing first diagram source; got:\n%s", out)
	}
	if !strings.Contains(out, "sequenceDiagram; X-&gt;&gt;Y: hi") {
		t.Fatalf("missing second diagram source; got:\n%s", out)
	}
}

func TestMermaid_MixedWithRegularCode(t *testing.T) {
	ctx := context.Background()
	md := "# T\n\n" +
		"```mermaid\ngraph TD; A-->B\n```\n\n" +
		"```go\nfunc main() {}\n```\n"
	out, err := RenderHTML(ctx, md, render.Options{Mermaid: true})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !strings.Contains(out, `<pre class="mermaid">`) {
		t.Fatalf("missing mermaid block; got:\n%s", out)
	}
	if !strings.Contains(out, `class="chroma`) {
		t.Fatalf("expected chroma-highlighted go block; got:\n%s", out)
	}
}

func TestMermaid_HTMLEscaping(t *testing.T) {
	ctx := context.Background()
	md := "```mermaid\n<script>alert(1)</script>\n```\n"
	out, err := RenderHTML(ctx, md, render.Options{Mermaid: true})
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !strings.Contains(out, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected escaped <script>; got:\n%s", out)
	}
	// Ensure the raw unescaped script tag from the source didn't leak through.
	// (The head script contains its own <script> tags — we only check the body
	// didn't contain a user-supplied one.)
	bodyStart := strings.Index(out, `<pre class="mermaid">`)
	if bodyStart < 0 {
		t.Fatalf("missing mermaid block; got:\n%s", out)
	}
	bodyEnd := strings.Index(out[bodyStart:], "</pre>")
	if bodyEnd < 0 {
		t.Fatalf("missing closing </pre>; got:\n%s", out)
	}
	body := out[bodyStart : bodyStart+bodyEnd]
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatalf("raw <script> leaked into mermaid block body: %s", body)
	}
}
