// Package math implements the :::math directive, which renders a raw
// LaTeX source block inside a .vaelor-math container.
//
// Syntax:
//
//	:::math
//	\int_{0}^{\infty} e^{-x^2} dx = \frac{\sqrt{\pi}}{2}
//	:::
//
// Body is treated as raw LaTeX and emitted HTML-escaped inside a
// <div class="vaelor-math"> container, styled as a math-like block.
//
// KaTeX rendering limitation (v1): full client-side KaTeX rendering
// requires injecting <script> tags into the document <head>, which the
// current directives framework does not expose (CSS() is the only
// per-directive theme hook). For v1 we ship a serif/italic math-styled
// block that reads as a math excerpt even without KaTeX. Full KaTeX
// support is deferred to a follow-up task that extends the Handler
// interface with a HeadHTML() (or equivalent) hook; when added, the
// runtime can inject the KaTeX CSS/JS CDN bundle and iterate
// .vaelor-math elements calling katex.render(el.textContent, el).
package math

import (
	_ "embed"
	"fmt"
	"html"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed math.css
var math_css []byte

// kindMathBlock is the AST node kind for a concrete math node produced
// by Transform. Renderer is wired under this kind.
var kindMathBlock = ast.NewNodeKind("VaelorMathBlock")

// node is the concrete AST node for a parsed :::math block.
type node struct {
	ast.BaseBlock

	// Source is the raw LaTeX body, copied from the generic block.
	Source []byte
}

// Kind implements ast.Node.
func (n *node) Kind() ast.NodeKind { return kindMathBlock }

// Dump implements ast.Node.
func (n *node) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// handler implements directives.Handler for the math directive.
type handler struct{}

// New returns a fresh math Handler. init() already registers a default
// instance; exposed for callers that want to register explicitly.
func New() directives.Handler { return handler{} }

// Name implements directives.Handler.
func (handler) Name() string { return "math" }

// Kind implements directives.Handler.
func (handler) Kind() ast.NodeKind { return kindMathBlock }

// CSS implements directives.Handler.
func (handler) CSS() []byte { return math_css }

// Transform implements directives.Handler. The body is preserved
// verbatim; it is LaTeX source, not markdown, so no sub-parsing runs.
func (handler) Transform(b *directives.Block, _ []byte, _ parser.Context) ast.Node {
	body := append([]byte(nil), b.Body...)
	return &node{Source: body}
}

// Render implements directives.Handler. It emits a .vaelor-math div
// containing the HTML-escaped LaTeX source. Escaping is mandatory to
// neutralize stray HTML/script content in the raw body; legitimate
// LaTeX escapes (e.g. \int, \frac) survive HTML escaping intact.
func (handler) Render(w util.BufWriter, _ []byte, raw ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n, ok := raw.(*node)
	if !ok {
		return ast.WalkContinue, nil
	}
	if _, err := fmt.Fprintf(w,
		`<div class="vaelor-math" data-katex-display>%s</div>`+"\n",
		html.EscapeString(string(n.Source)),
	); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

func init() {
	directives.Register(New())
}
