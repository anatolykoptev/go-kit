// Package timeline implements the :::timeline directive, which renders a
// vertical list of date/event pairs. Body lines match the pattern
//
//   - <date>: <event>
//
// where <date> is an opaque string (no format validation) and <event> is
// plain text (HTML-escaped on render). Lines that don't match are rendered
// as an event with an empty date, accompanied by a log warning.
package timeline

import (
	_ "embed"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"

	"github.com/anatolykoptev/go-kit/render/html/directives"
)

//go:embed timeline.css
var timelineCSS []byte

// lineRE splits a body line into date (group 1) and event (group 2). Only
// the first colon separates — subsequent colons remain in the event text.
// Leading `-` and surrounding whitespace are tolerated.
var lineRE = regexp.MustCompile(`^-\s*(.+?)\s*:\s*(.+?)\s*$`)

// kindTimeline is the AST node kind for a parsed :::timeline block.
var kindTimeline = ast.NewNodeKind("VaelorTimelineBlock")

// entry is one date/event pair rendered as a single <li>.
type entry struct {
	Date  string
	Event string
}

// timelineNode is the concrete AST node produced by Handler.Transform.
type timelineNode struct {
	ast.BaseBlock
	Entries []entry
}

// Kind implements ast.Node.
func (n *timelineNode) Kind() ast.NodeKind { return kindTimeline }

// Dump implements ast.Node.
func (n *timelineNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{
		"Entries": strconv.Itoa(len(n.Entries)),
	}, nil)
}

// Handler implements directives.Handler for :::timeline.
type Handler struct{}

// New returns a Handler suitable for registration.
func New() Handler { return Handler{} }

// Name implements directives.Handler.
func (Handler) Name() string { return "timeline" }

// Kind implements directives.Handler.
func (Handler) Kind() ast.NodeKind { return kindTimeline }

// CSS implements directives.Handler.
func (Handler) CSS() []byte { return timelineCSS }

// Transform parses the body into entries and returns a *timelineNode.
func (Handler) Transform(b *directives.Block, _ []byte, _ parser.Context) ast.Node {
	entries := parseBody(b.Body)
	return &timelineNode{Entries: entries}
}

// parseBody splits b into entries. Non-empty lines that don't start with
// `-` are skipped. Lines starting with `-` that don't match lineRE are
// treated as an event with an empty date, and a warning is logged.
func parseBody(body []byte) []entry {
	text := string(body)
	lines := strings.Split(text, "\n")
	entries := make([]entry, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "-") {
			// Not a timeline entry line; skip silently (comment/blank).
			continue
		}
		if m := lineRE.FindStringSubmatch(line); m != nil {
			entries = append(entries, entry{
				Date:  strings.TrimSpace(m[1]),
				Event: strings.TrimSpace(m[2]),
			})
			continue
		}
		// Dash line without a colon — malformed. Render as event-only.
		ev := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		log.Printf("timeline: malformed line (no date separator): %q", line)
		entries = append(entries, entry{Date: "", Event: ev})
	}
	return entries
}

// Render emits the <ol>...</ol> structure. Called for both entering and
// exiting; we emit everything on entering and skip children.
func (Handler) Render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	node, ok := n.(*timelineNode)
	if !ok {
		return ast.WalkContinue, nil
	}
	if len(node.Entries) == 0 {
		if _, err := w.WriteString(`<ol class="vaelor-timeline"></ol>`); err != nil {
			return ast.WalkStop, fmt.Errorf("timeline: write empty: %w", err)
		}
		return ast.WalkSkipChildren, nil
	}
	if _, err := w.WriteString(`<ol class="vaelor-timeline">`); err != nil {
		return ast.WalkStop, fmt.Errorf("timeline: write open: %w", err)
	}
	for _, e := range node.Entries {
		if _, err := fmt.Fprintf(
			w,
			`<li class="vaelor-timeline-item"><time class="vaelor-timeline-date">%s</time><span class="vaelor-timeline-event">%s</span></li>`,
			html.EscapeString(e.Date),
			html.EscapeString(e.Event),
		); err != nil {
			return ast.WalkStop, fmt.Errorf("timeline: write entry: %w", err)
		}
	}
	if _, err := w.WriteString(`</ol>`); err != nil {
		return ast.WalkStop, fmt.Errorf("timeline: write close: %w", err)
	}
	return ast.WalkSkipChildren, nil
}

func init() {
	directives.Register(New())
}
