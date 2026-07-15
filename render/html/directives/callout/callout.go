// Package callout implements the :::callout{type=... title="..."} directive,
// which renders an <aside> with a typed left border and optional heading.
// Supported types: info (default), warning, error, success.
package callout

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed callout.css
var callout_css []byte

// kindCallout is the AST node kind for a concrete callout node produced by
// Transform. Renderer is wired under this kind.
var kindCallout = ast.NewNodeKind("VaelorCalloutBlock")

// validTypes enumerates recognized callout types. Unknown values fall back
// to "info".
var validTypes = map[string]struct{}{
	"info":    {},
	"warning": {},
	"error":   {},
	"success": {},
}

// node is the concrete AST node for a parsed :::callout{...} block.
// CalloutType (not Type) avoids shadowing ast.Node.Type().
type node struct {
	ast.BaseBlock

	// CalloutType is one of info/warning/error/success (always valid by construction).
	CalloutType string

	// Title is the optional title text; empty when absent.
	Title string

	// Body is the raw markdown body; rendered via a fresh goldmark.
	Body []byte
}

// Kind implements ast.Node.
func (n *node) Kind() ast.NodeKind { return kindCallout }

// Dump implements ast.Node.
func (n *node) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{
		"Type":  n.CalloutType,
		"Title": n.Title,
	}, nil)
}

// handler implements directives.Handler for the callout directive.
type handler struct{}

// New returns a fresh callout Handler. Exposed for callers that want to
// register explicitly; init() already registers a default instance.
func New() directives.Handler { return handler{} }

// Name implements directives.Handler.
func (handler) Name() string { return "callout" }

// Kind implements directives.Handler.
func (handler) Kind() ast.NodeKind { return kindCallout }

// CSS implements directives.Handler.
func (handler) CSS() []byte { return callout_css }

// Transform implements directives.Handler. It reads the type/title attrs
// from the generic block and returns a callout node containing the body.
func (handler) Transform(b *directives.Block, _ []byte, _ parser.Context) ast.Node {
	t := b.Attrs["type"]
	if _, ok := validTypes[t]; !ok {
		t = "info"
	}
	title := b.Attrs["title"]
	body := append([]byte(nil), b.Body...)
	return &node{
		CalloutType: t,
		Title:       title,
		Body:        body,
	}
}

// Render implements directives.Handler. It emits a fully-formed <aside>
// wrapping the body, which is re-parsed through a fresh goldmark (no
// directive extension, to prevent infinite recursion).
func (handler) Render(w util.BufWriter, _ []byte, raw ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := raw.(*node)
	if !ok {
		return ast.WalkContinue, nil
	}

	bodyHTML, err := renderBody(n.Body)
	if err != nil {
		return ast.WalkStop, fmt.Errorf("callout: render body: %w", err)
	}

	if _, err := fmt.Fprintf(w,
		`<aside class="vaelor-callout vaelor-callout--%s">`,
		html.EscapeString(n.CalloutType),
	); err != nil {
		return ast.WalkStop, err
	}
	if n.Title != "" {
		if _, err := fmt.Fprintf(w,
			`<h4 class="vaelor-callout-title">%s</h4>`,
			html.EscapeString(n.Title),
		); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := fmt.Fprintf(w,
		`<div class="vaelor-callout-body">%s</div></aside>`,
		bodyHTML,
	); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

// renderBody parses the inner markdown body through a fresh goldmark
// instance. The directives extension is deliberately NOT added here to
// prevent infinite recursion on nested :::callout blocks.
func renderBody(src []byte) (string, error) {
	gm := goldmark.New(goldmark.WithExtensions(extension.GFM))
	var buf bytes.Buffer
	if err := gm.Convert(src, &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func init() {
	directives.Register(New())
}
