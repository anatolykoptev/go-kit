package html

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
)

// tocEntry is one heading captured for table-of-contents rendering.
type tocEntry struct {
	level int    // 1-6
	text  string // plain text, HTML-escaped when written
	id    string // slug for href="#..."
}

// collectHeadings walks the AST and returns all headings with slugified IDs.
// Duplicate slugs are disambiguated with -2, -3 suffixes.
func collectHeadings(doc ast.Node, source []byte) []tocEntry {
	var out []tocEntry
	seen := map[string]int{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		txt := inlineText(h, source)
		id := slugify(txt)
		if id == "" {
			id = fmt.Sprintf("heading-%d", len(out)+1)
		}
		if seen[id] > 0 {
			seen[id]++
			id = fmt.Sprintf("%s-%d", id, seen[id])
		} else {
			seen[id] = 1
		}
		out = append(out, tocEntry{level: h.Level, text: txt, id: id})
		return ast.WalkContinue, nil
	})
	return out
}

var (
	slugifyNonSafe = regexp.MustCompile(`[^a-z0-9-]+`)
	slugifyDashes  = regexp.MustCompile(`-+`)
)

// slugify converts heading text into a URL-safe id. Single source of truth for
// both TOC hrefs and injected heading IDs so the two always line up.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugifyNonSafe.ReplaceAllString(s, "")
	s = slugifyDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// renderTOC returns an HTML string containing the TOC nav. Returns "" when the
// document has fewer than 2 headings (heuristic: single heading doesn't benefit).
func renderTOC(entries []tocEntry) string {
	if len(entries) < 2 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="toc"><h2>Table of Contents</h2><ul>`)
	for _, e := range entries {
		fmt.Fprintf(&b, `<li class="toc-h%d"><a href="#%s">%s</a></li>`,
			e.level, html.EscapeString(e.id), html.EscapeString(e.text))
	}
	b.WriteString(`</ul></nav>`)
	return b.String()
}

var headingTag = regexp.MustCompile(`<(h[1-6])>`)

// injectHeadingIDs adds id="<slug>" to each <h1>-<h6> opening tag in the rendered HTML.
// Uses entries in document order (goldmark emits headings in source order).
func injectHeadingIDs(body string, entries []tocEntry) string {
	i := 0
	return headingTag.ReplaceAllStringFunc(body, func(m string) string {
		if i >= len(entries) {
			return m
		}
		tag := strings.TrimPrefix(strings.TrimSuffix(m, ">"), "<")
		result := fmt.Sprintf(`<%s id="%s">`, tag, html.EscapeString(entries[i].id))
		i++
		return result
	})
}
