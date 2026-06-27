// Package stats implements the :::stats{...} directive. The body is
// parsed as a YAML list of items with {label, value, delta?} keys and
// rendered as a grid of stat cards with delta color-coding.
package stats

import (
	_ "embed"
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed stats.css
var css []byte

// kindStats is the AST node kind for a parsed :::stats block.
var kindStats = ast.NewNodeKind("VaelorStatsBlock")

// Item is one stat card. Extra YAML keys are ignored by the decoder.
type Item struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
	Delta string `yaml:"delta"`
}

// node is the handler-specific AST node. When Err is non-empty the
// renderer emits an error div instead of the grid.
type node struct {
	ast.BaseBlock
	Items []Item
	Err   string
}

func (*node) Kind() ast.NodeKind       { return kindStats }
func (n *node) Dump(s []byte, lvl int) { ast.DumpHelper(n, s, lvl, nil, nil) }

// handler implements directives.Handler for :::stats.
type handler struct{}

// New returns a fresh stats handler. Exported for test construction.
func New() directives.Handler { return handler{} }

func (handler) Name() string       { return "stats" }
func (handler) Kind() ast.NodeKind { return kindStats }
func (handler) CSS() []byte        { return css }

// Transform parses the body YAML into a typed node. Errors are
// preserved on the node so Render can emit an error div.
func (handler) Transform(b *directives.Block, _ []byte, _ parser.Context) ast.Node {
	var items []Item
	if err := yaml.Unmarshal(b.Body, &items); err != nil {
		return &node{Err: "stats: expected YAML list"}
	}
	return &node{Items: items}
}

// Render emits the stats HTML. Delta coloring class is resolved here
// so the transform stays pure-data.
func (handler) Render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	sn, ok := n.(*node)
	if !ok {
		return ast.WalkContinue, nil
	}
	if sn.Err != "" {
		_, _ = fmt.Fprintf(w, `<div class="vaelor-directive-error">%s</div>`, html.EscapeString(sn.Err))
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(`<div class="vaelor-stats">`)
	for _, it := range sn.Items {
		_, _ = w.WriteString(`<div class="vaelor-stat">`)
		_, _ = fmt.Fprintf(w, `<div class="vaelor-stat-value">%s</div>`, html.EscapeString(it.Value))
		_, _ = fmt.Fprintf(w, `<div class="vaelor-stat-label">%s</div>`, html.EscapeString(it.Label))
		if it.Delta != "" {
			_, _ = fmt.Fprintf(w,
				`<div class="vaelor-stat-delta %s">%s</div>`,
				deltaClass(it.Delta),
				html.EscapeString(it.Delta),
			)
		}
		_, _ = w.WriteString(`</div>`)
	}
	_, _ = w.WriteString(`</div>`)
	return ast.WalkSkipChildren, nil
}

// deltaClass returns the CSS modifier class for a delta string based
// on its leading sign character.
func deltaClass(delta string) string {
	switch {
	case strings.HasPrefix(delta, "+"):
		return "vaelor-stat-delta--up"
	case strings.HasPrefix(delta, "-"):
		return "vaelor-stat-delta--down"
	default:
		return "vaelor-stat-delta--neutral"
	}
}

func init() {
	directives.Register(New())
}
