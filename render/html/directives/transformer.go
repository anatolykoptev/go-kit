package directives

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// transformer walks the parsed AST, finds every generic directive *Block,
// and dispatches it to the registered Handler for its Name. The Handler's
// returned node replaces the generic Block in-place.
//
// Unclosed blocks and blocks with no registered handler are kept in the
// tree as generic *Block nodes; the renderer emits appropriate fallback
// markup (error paragraph / HTML comment) for them.
type transformer struct{}

// Transform implements parser.ASTTransformer.
func (transformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	src := reader.Source()
	var blocks []*Block
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if b, ok := n.(*Block); ok {
			blocks = append(blocks, b)
		}
		return ast.WalkContinue, nil
	})
	for _, b := range blocks {
		if !b.Closed {
			// Leave generic *Block — renderer emits the error paragraph.
			continue
		}
		h, ok := lookup(b.Name)
		if !ok {
			// Leave generic *Block — renderer emits HTML comment.
			continue
		}
		replacement := h.Transform(b, src, pc)
		if replacement == nil {
			continue
		}
		parent := b.Parent()
		if parent == nil {
			continue
		}
		parent.ReplaceChild(parent, b, replacement)
	}
}
