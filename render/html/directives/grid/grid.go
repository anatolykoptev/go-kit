// Package grid implements the :::grid{cols=N} directive. The body is
// re-parsed as markdown (fresh goldmark instance, no directive recursion)
// and top-level block children are distributed round-robin across N columns.
package grid

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed grid.css
var gridCSS []byte

const (
	minCols     = 1
	maxCols     = 4
	defaultCols = 2
)

// kindGrid is the AST node kind for the grid directive.
var kindGrid = ast.NewNodeKind("VaelorGridBlock")

// node is the handler-specific AST node. It carries the resolved column
// count plus the pre-rendered HTML for each column, computed at Transform
// time so Render can just stream the strings out.
type node struct {
	ast.BaseBlock
	Cols    int
	Columns []string
}

func (n *node) Kind() ast.NodeKind { return kindGrid }
func (n *node) Dump(src []byte, l int) {
	ast.DumpHelper(n, src, l, map[string]string{
		"Cols": strconv.Itoa(n.Cols),
	}, nil)
}

// Handler implements directives.Handler for :::grid{...}.
type Handler struct{}

// New returns a registered-ready Handler instance.
func New() Handler { return Handler{} }

// Name implements directives.Handler.
func (Handler) Name() string { return "grid" }

// Kind implements directives.Handler.
func (Handler) Kind() ast.NodeKind { return kindGrid }

// CSS implements directives.Handler.
func (Handler) CSS() []byte { return gridCSS }

// Transform implements directives.Handler. It resolves the column count,
// re-parses the body with a fresh goldmark (GFM, no directives) and
// distributes the resulting top-level blocks round-robin across N columns.
func (Handler) Transform(b *directives.Block, _ []byte, _ parser.Context) ast.Node {
	cols := resolveCols(b.Attrs["cols"])
	columns := distribute(b.Body, cols)
	return &node{Cols: cols, Columns: columns}
}

// resolveCols parses the cols attribute and clamps it to [minCols,maxCols].
// Missing or non-integer values fall back to defaultCols.
func resolveCols(raw string) int {
	if raw == "" {
		return defaultCols
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultCols
	}
	if n < minCols {
		return minCols
	}
	if n > maxCols {
		return maxCols
	}
	return n
}

// distribute re-parses body as markdown and returns cols HTML strings,
// each containing the round-robin-assigned top-level blocks for that
// column. Returns exactly cols entries (possibly empty strings).
func distribute(body []byte, cols int) []string {
	out := make([]string, cols)
	if cols <= 0 || len(bytes.TrimSpace(body)) == 0 {
		return out
	}

	// Fresh goldmark without the directives extension — avoids recursion.
	gm := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader(body)
	doc := gm.Parser().Parse(reader)

	// Collect immediate children, since Render mutates sibling traversal.
	var children []ast.Node
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		children = append(children, c)
	}

	buffers := make([]bytes.Buffer, cols)
	for i, child := range children {
		col := i % cols
		// Render the single node. Goldmark's Render walks the node + its
		// descendants; calling it on a subtree works because the default
		// HTML renderer writes entering/leaving markup per node without
		// depending on parent context.
		if err := gm.Renderer().Render(&buffers[col], body, child); err != nil {
			// On render failure, inject an HTML comment so the column is
			// not silently dropped. This should be unreachable for
			// well-formed markdown.
			fmt.Fprintf(&buffers[col], "<!-- grid: render error: %v -->\n", err)
		}
	}
	for i := range buffers {
		out[i] = buffers[i].String()
	}
	return out
}

// Render implements directives.Handler. It emits the wrapper div with the
// --cols CSS variable and one .vaelor-grid-col div per column.
func (Handler) Render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	g, ok := n.(*node)
	if !ok {
		return ast.WalkContinue, nil
	}
	if _, err := fmt.Fprintf(w, `<div class="vaelor-grid" style="--cols:%d">`, g.Cols); err != nil {
		return ast.WalkStop, err
	}
	for _, colHTML := range g.Columns {
		if _, err := w.WriteString(`<div class="vaelor-grid-col">`); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.WriteString(colHTML); err != nil {
			return ast.WalkStop, err
		}
		if _, err := w.WriteString(`</div>`); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := w.WriteString(`</div>` + "\n"); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}

func init() {
	directives.Register(New())
}
