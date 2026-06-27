package directives

import (
	"fmt"
	"html"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// nodeRenderer implements renderer.NodeRenderer for the framework. It
// registers a single function for kindBlock (the generic *Block) which
// handles the three fallback cases: unclosed blocks, unknown directives,
// and — when a handler IS registered but somehow didn't replace the node
// — delegates to the handler's Render.
//
// Concrete handler-owned kinds are registered separately by looking up
// every registered Handler and wiring Handler.Render for Handler.Kind().
type nodeRenderer struct{}

// RegisterFuncs implements renderer.NodeRenderer.
func (nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindBlock, renderGenericBlock)
	// Register each handler's render function under its own Kind.
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, h := range handlers {
		h := h
		reg.Register(h.Kind(), h.Render)
	}
}

// renderGenericBlock handles *Block nodes that survive the AST-transform
// pass — i.e. unclosed directives (error) and unknown directive names
// (HTML comment fallback).
func renderGenericBlock(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	b, ok := n.(*Block)
	if !ok {
		return ast.WalkContinue, nil
	}
	if !b.Closed {
		// Error fallback — unclosed directive at EOF.
		_, _ = fmt.Fprintf(w,
			`<p class="vaelor-directive-error">Unclosed directive: %s (missing :::)</p>`+"\n",
			html.EscapeString(b.Name),
		)
		return ast.WalkSkipChildren, nil
	}
	// Unknown directive — closed but no registered handler.
	_, _ = fmt.Fprintf(w, "<!-- unknown directive: %s -->\n", html.EscapeString(b.Name))
	return ast.WalkSkipChildren, nil
}
