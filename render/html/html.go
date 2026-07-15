// Package html converts markdown content to styled HTML suitable for
// headless-Chrome PDF printing.
package html

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/anatolykoptev/go-kit/render"
	"github.com/anatolykoptev/go-kit/render/html/directives"
	// Blank import forces every directive subpackage's init() to run so
	// handlers self-register with the directives registry.
	_ "github.com/anatolykoptev/go-kit/render/html/directives/all"
)

// defaultTitle is used when no explicit Title option is provided and no H1
// heading is present in the markdown source.
const defaultTitle = "Document"

// shellTemplate is parsed lazily on first use and cached.
var shellTemplate = template.Must(template.New("shell").Parse(shellHTML))

// shellData carries values into the HTML shell template. CSS, HeadExtras,
// PreBody and Body are pre-rendered trusted content (our embedded stylesheet,
// goldmark HTML output, hook output), so they are passed as
// template.CSS / template.HTML to bypass auto-escaping. Title is passed as a
// plain string and is auto-escaped by html/template.
type shellData struct {
	Title      string
	CSS        template.CSS
	HeadExtras template.HTML // hook for <head> inserts (mermaid script, etc.)
	PreBody    template.HTML // hook for pre-body inserts (cover page, etc.)
	Body       template.HTML
}

// buildGoldmark constructs the goldmark markdown parser/renderer with the
// extensions we always enable, plus any extensions registered via
// opts (mermaid, etc.). Exposed as a helper so callers can plug in
// without modifying RenderHTML.
func buildGoldmark(opts render.Options) goldmark.Markdown {
	gmOpts := []goldmark.Option{
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithStyle(chromaStyleName),
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
	}
	registerMermaidExtension(&gmOpts, opts)
	if opts.Directives {
		gmOpts = append(gmOpts, goldmark.WithExtensions(directives.Extension()))
	}
	return goldmark.New(gmOpts...)
}

// applyOptions builds a normalized Options from the variadic slice.
// Unset fields get defaults: Theme defaults to "report".
func applyOptions(opts []render.Option) render.Options {
	out := render.Options{Theme: "report"}
	for _, o := range opts {
		o(&out)
	}
	if out.Theme == "" {
		out.Theme = "report"
	}
	return out
}

// RenderHTML converts markdown to a complete HTML document ready for
// headless-Chrome PDF printing.
func RenderHTML(ctx context.Context, markdown string, opts render.Options) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", errors.New("markdown is empty")
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("context cancelled: %w", err)
	}

	source := []byte(markdown)

	md := buildGoldmark(opts)

	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// image embedding
	if opts.ImageEmbed.Enabled {
		if err := embedImages(ctx, doc, source, opts.ImageEmbed); err != nil {
			return "", fmt.Errorf("embed images: %w", err)
		}
	}

	title := opts.Title
	if title == "" {
		title = extractFirstH1(doc, source)
	}
	if title == "" {
		title = defaultTitle
	}

	var body bytes.Buffer
	if err := md.Renderer().Render(&body, source, doc); err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}

	bodyStr := body.String()
	if opts.TOC {
		entries := collectHeadings(doc, source)
		bodyStr = injectHeadingIDs(bodyStr, entries)
		bodyStr = renderTOC(entries) + bodyStr
	}

	css := LookupTheme(opts.Theme)
	if opts.CustomCSS != "" {
		css = css + "\n" + opts.CustomCSS
	}

	// hooks: cover page & mermaid head
	var preBody, headExtras string
	if opts.CoverPage != nil {
		preBody = renderCoverPage(opts.CoverPage)
	}
	if opts.Mermaid {
		headExtras = mermaidHeadScript()
	}

	var out bytes.Buffer
	data := shellData{
		Title:      title,
		CSS:        template.CSS(css),         //nolint:gosec // trusted: embedded preset + caller CSS
		HeadExtras: template.HTML(headExtras), //nolint:gosec // trusted: hook output (always empty when Mermaid=false)
		PreBody:    template.HTML(preBody),    //nolint:gosec // trusted: hook output (always empty when CoverPage=nil)
		Body:       template.HTML(bodyStr),    //nolint:gosec // trusted: goldmark HTML output + our TOC
	}
	if err := shellTemplate.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute shell template: %w", err)
	}

	return out.String(), nil
}

// RenderHTMLWith is a variadic-option convenience that wraps RenderHTML.
// New code should prefer this form.
func RenderHTMLWith(ctx context.Context, markdown string, opts ...render.Option) (string, error) {
	return RenderHTML(ctx, markdown, applyOptions(opts))
}
