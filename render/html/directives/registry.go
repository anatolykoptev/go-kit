package directives

import (
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/util"
)

// Handler is implemented by each concrete directive (callout, stats, etc.).
type Handler interface {
	// Name is the directive identifier used in :::name{...} syntax.
	Name() string

	// Transform replaces the generic *Block with a concrete AST node
	// representing this directive. Runs during the AST-transform phase.
	Transform(block *Block, src []byte, pc parser.Context) ast.Node

	// Kind is the ast.NodeKind of the node returned by Transform.
	Kind() ast.NodeKind

	// Render emits HTML for nodes of Kind().
	Render(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error)

	// CSS returns scoped CSS bytes for this directive; called by the theme
	// loader to produce the aggregated directives.css content.
	CSS() []byte
}

// registryMu guards the handlers map. Package-private because subpackages
// mutate the registry only via Register (which takes the lock).
var registryMu sync.RWMutex

// handlers is the global directive registry keyed by Handler.Name().
var handlers = map[string]Handler{}

// Register installs a Handler so the framework dispatches :::<name>{...}
// blocks to it. Safe for concurrent use. Duplicate names overwrite the
// previous registration, which matches the init() ordering contract of
// Go — last package to init() wins, but for our blank-import aggregator
// each directive has a unique name so collisions indicate a bug.
func Register(h Handler) {
	if h == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	handlers[h.Name()] = h
}

// lookup returns the handler registered for name and whether it exists.
func lookup(name string) (Handler, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	h, ok := handlers[name]
	return h, ok
}

// reset clears the registry. Test-only helper exposed through resetForTest.
func reset() {
	registryMu.Lock()
	defer registryMu.Unlock()
	handlers = map[string]Handler{}
}
