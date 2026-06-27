package html

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// extractFirstH1 walks the document looking for the first level-1 heading and
// returns its plain text. Returns "" if no H1 is present.
func extractFirstH1(doc ast.Node, source []byte) string {
	var title string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level != 1 {
			return ast.WalkContinue, nil
		}
		title = strings.TrimSpace(inlineText(h, source))
		return ast.WalkStop, nil
	})
	return title
}

// inlineText returns the concatenated plain-text content of all descendant
// ast.Text nodes under n. Replaces the deprecated ast.Node.Text method and
// correctly handles inline code spans, emphasis, and other inline wrappers
// within headings.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}
