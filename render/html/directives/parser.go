package directives

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// openRE matches a directive opening fence on its own line:
//
//	:::name
//	:::name{attrs...}
//
// Trailing whitespace is tolerated. The name must start with a lowercase
// letter and contain only [a-z0-9_-].
var openRE = regexp.MustCompile(`^:::([a-z][a-z0-9_-]*)(\{[^}]*\})?[ \t]*$`)

// closeRE matches a bare closing fence: `:::` (optional trailing whitespace).
var closeRE = regexp.MustCompile(`^:::[ \t]*$`)

// blockParser implements parser.BlockParser for :::name{...} directive
// blocks. It parses greedily line-by-line, treating inner `:::` lines as
// body text (no nesting in v1) and closing on the first bare `:::` line.
type blockParser struct{}

func newBlockParser() parser.BlockParser { return blockParser{} }

// Trigger reports that the parser runs only on lines starting with ':'.
func (blockParser) Trigger() []byte { return []byte{':'} }

// Open consumes the opening fence line when it matches openRE. It creates
// a *Block and returns NoChildren — the body is accumulated in Continue.
func (blockParser) Open(_ ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	stripped := bytes.TrimRight(line, "\r\n")
	m := openRE.FindSubmatch(stripped)
	if m == nil {
		return nil, parser.NoChildren
	}
	block := &Block{
		Name:  string(m[1]),
		Attrs: ParseAttrs(string(m[2])),
		Raw:   append([]byte(nil), stripped...),
	}
	return block, parser.NoChildren
}

// Continue accumulates body lines until a matching `:::` closing fence is
// found. On close it marks the block Closed=true and returns Close.
func (blockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	block, ok := node.(*Block)
	if !ok {
		return parser.Close
	}
	line, segment := reader.PeekLine()
	if line == nil {
		// EOF with no closing fence — signal Close; Close() records unclosed.
		return parser.Close
	}
	stripped := bytes.TrimRight(line, "\r\n")
	if closeRE.Match(stripped) {
		block.Closed = true
		reader.AdvanceToEOL()
		return parser.Close
	}
	// Append line segment to the block via ast.Lines for later body reconstruction.
	block.Lines().Append(segment)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

// Close reconstructs the block's Body bytes from its accumulated line
// segments. Doing this in Close gives us a stable []byte slice to hand to
// handlers without them needing to know about goldmark segments.
func (blockParser) Close(node ast.Node, reader text.Reader, _ parser.Context) {
	block, ok := node.(*Block)
	if !ok {
		return
	}
	src := reader.Source()
	lines := block.Lines()
	var buf bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
	}
	block.Body = buf.Bytes()
	if lines.Len() > 0 {
		block.BodyLine = lines.At(0).Start
	}
}

// CanInterruptParagraph allows a `:::name` line to start a new block even
// when it follows a paragraph without a blank line in between.
func (blockParser) CanInterruptParagraph() bool { return true }

// CanAcceptIndentedLine is false: we require 0 indentation in v1.
func (blockParser) CanAcceptIndentedLine() bool { return false }
