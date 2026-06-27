package directives

import "github.com/yuin/goldmark/ast"

// kindBlock is the AST node kind for a generic directive block produced by
// the parser. The transformer replaces these with handler-specific nodes.
var kindBlock = ast.NewNodeKind("VaelorDirectiveBlock")

// Block is the generic AST node emitted by the parser for any :::name{...}
// directive. The transformer dispatches this to the registered Handler for
// Name and replaces it with a handler-specific node.
type Block struct {
	ast.BaseBlock

	// Name is the directive identifier from :::name{...}.
	Name string

	// Attrs holds parsed key=value attributes.
	Attrs map[string]string

	// Body is the raw directive body (lines between opening and closing
	// fences), joined with their trailing newlines intact.
	Body []byte

	// BodyLine is the 1-based source line on which the body starts (the
	// line after the opening fence). Used for error reporting.
	BodyLine int

	// Closed is true when the parser encountered a matching `:::` closing
	// fence. Unclosed blocks are rendered as an error node.
	Closed bool

	// Raw holds the original opening-fence text, preserved so error
	// rendering can show the exact source.
	Raw []byte
}

// Kind implements ast.Node.
func (b *Block) Kind() ast.NodeKind { return kindBlock }

// Dump implements ast.Node.
func (b *Block) Dump(src []byte, level int) {
	ast.DumpHelper(b, src, level, map[string]string{
		"Name":   b.Name,
		"Closed": boolStr(b.Closed),
	}, nil)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
