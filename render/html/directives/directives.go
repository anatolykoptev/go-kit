// Package directives implements the ::: directive-block framework for
// go-kit's HTML/PDF renderer. Directive blocks use the syntax:
//
//	:::name{key=value key2="quoted value" flag}
//	... body lines ...
//	:::
//
// The package is split into a framework (this package) and per-directive
// subpackages (callout, stats, grid, timeline). Each subpackage registers
// itself via an init() that calls Register. The aggregator subpackage
// directives/all blank-imports every subpackage so that a single import
// wires the full set of built-in directives.
//
// Phase 1 ships the framework with only an internal test-only "echo"
// directive; Phase 2 agents fill in the real directive subpackages.
package directives

import (
	"sort"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// Extension returns a goldmark.Extender that wires the directive parser,
// AST transformer, and renderer into a goldmark instance.
func Extension() goldmark.Extender {
	return extension{}
}

// extension implements goldmark.Extender for the directives framework.
type extension struct{}

// Extend implements goldmark.Extender.
func (extension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(newBlockParser(), 199),
	))
	m.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(transformer{}, 800),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(nodeRenderer{}, 100),
	))
}

// AllCSS concatenates every registered Handler's CSS() return, in Name()
// order, separated by newlines. Safe to call even when no handler is
// registered (returns nil).
func AllCSS() []byte {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if len(handlers) == 0 {
		return nil
	}
	names := make([]string, 0, len(handlers))
	for name := range handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []byte
	for _, name := range names {
		css := handlers[name].CSS()
		if len(css) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, '\n')
		}
		out = append(out, css...)
	}
	return out
}
