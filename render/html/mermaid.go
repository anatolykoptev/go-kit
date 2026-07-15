package html

import (
	"bytes"
	"fmt"
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	render "github.com/anatolykoptev/go-kit/render"
)

// Mermaid is pinned to a specific version with an SRI integrity hash so a
// future CDN compromise cannot inject arbitrary JS into rendered PDFs.
// To update: compute sha384 of the new release's dist/mermaid.min.js, e.g.
//
//	curl -s https://cdn.jsdelivr.net/npm/mermaid@<v>/dist/mermaid.min.js \
//	    | openssl dgst -sha384 -binary | openssl base64 -A
const (
	mermaidVersion   = "11.9.0"
	mermaidIntegrity = "sha384-UzWEhMP22MxNnr2bzqAdmtf1FDy5iKDUq6hLXJFLqC7dfGkc6W/hshbx9m71zyt5"
)

// mermaidHeadScript returns the <head>-injection HTML that loads mermaid
// from a CDN and initializes it. The caller is responsible for gating on
// opts.Mermaid before including it in the document.
func mermaidHeadScript() string {
	return fmt.Sprintf(
		`<script src="https://cdn.jsdelivr.net/npm/mermaid@%s/dist/mermaid.min.js" `+
			`integrity="%s" crossorigin="anonymous"></script>`+
			`<script>mermaid.initialize({startOnLoad: true, theme: 'default'});</script>`,
		mermaidVersion, mermaidIntegrity)
}

// registerMermaidExtension plugs the mermaid extension into the goldmark
// option slice. When opts.Mermaid is false the function is a no-op so the
// default highlighting of ```mermaid fences continues unchanged.
func registerMermaidExtension(gmOpts *[]goldmark.Option, opts render.Options) {
	if !opts.Mermaid {
		return
	}
	*gmOpts = append(*gmOpts, goldmark.WithExtensions(mermaidExtension{}))
}

// kindMermaidBlock is the AST node kind for a mermaid code block produced by
// the transformer.
var kindMermaidBlock = ast.NewNodeKind("MermaidBlock")

// mermaidBlock is an AST node carrying the raw mermaid diagram source.
type mermaidBlock struct {
	ast.BaseBlock
	Source []byte
}

// Kind implements ast.Node.
func (m *mermaidBlock) Kind() ast.NodeKind { return kindMermaidBlock }

// Dump implements ast.Node.
func (m *mermaidBlock) Dump(src []byte, level int) {
	ast.DumpHelper(m, src, level, nil, nil)
}

// mermaidASTTransformer walks the parsed AST and replaces fenced code blocks
// whose info string is "mermaid" with a mermaidBlock node. Running as a
// transformer means highlighting never sees these fences.
type mermaidASTTransformer struct{}

// Transform implements parser.ASTTransformer.
func (mermaidASTTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var toReplace []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fc, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		if string(fc.Language(src)) == "mermaid" {
			toReplace = append(toReplace, fc)
		}
		return ast.WalkContinue, nil
	})
	for _, fc := range toReplace {
		var buf bytes.Buffer
		lines := fc.Lines()
		for i := 0; i < lines.Len(); i++ {
			line := lines.At(i)
			buf.Write(line.Value(src))
		}
		mb := &mermaidBlock{Source: append([]byte(nil), buf.Bytes()...)}
		parent := fc.Parent()
		if parent == nil {
			continue
		}
		parent.ReplaceChild(parent, fc, mb)
	}
}

// mermaidNodeRenderer emits <pre class="mermaid">...</pre> for mermaidBlock
// nodes. Mermaid.js (loaded via the head script) reads these elements on page
// load and replaces them with rendered SVG.
type mermaidNodeRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (mermaidNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMermaidBlock, renderMermaidBlock)
}

func renderMermaidBlock(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	mb, ok := n.(*mermaidBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	_, _ = fmt.Fprintf(w, `<pre class="mermaid">%s</pre>%s`, html.EscapeString(string(mb.Source)), "\n")
	return ast.WalkSkipChildren, nil
}

// mermaidExtension wires the transformer and the renderer into a goldmark
// instance.
type mermaidExtension struct{}

// Extend implements goldmark.Extender.
func (mermaidExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(mermaidASTTransformer{}, 999),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(mermaidNodeRenderer{}, 100),
	))
}
