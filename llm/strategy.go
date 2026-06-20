package llm

import (
	"log/slog"
	"math/rand"
)

// SelectionStrategy controls the order in which eligible chain endpoints are tried.
type SelectionStrategy int

const (
	// SelectionPriority tries endpoints in configured order (primary first, then
	// fallbacks). This is the default — identical to pre-feature behaviour.
	SelectionPriority SelectionStrategy = iota
	// SelectionRandom shuffles eligible (healthy + non-cooled) endpoints on each
	// request before the try-loop. No single endpoint is always hammered first;
	// load is distributed across the pool.
	SelectionRandom
)

// parseSelectionStrategy converts an env-var string value to a SelectionStrategy.
// Unknown / empty values log a warning and fall back to SelectionPriority.
func parseSelectionStrategy(s string) SelectionStrategy {
	switch s {
	case "random":
		return SelectionRandom
	case "priority", "":
		return SelectionPriority
	default:
		slog.Warn("llm: unknown LLM_SELECTION_STRATEGY value, using priority", "value", s)
		return SelectionPriority
	}
}

// shuffleEndpoints returns a shuffled COPY of eps using r (or the global rand
// source when r is nil). Never mutates the input slice.
func shuffleEndpoints(eps []Endpoint, r *rand.Rand) []Endpoint {
	out := make([]Endpoint, len(eps))
	copy(out, eps)
	if r != nil {
		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	} else {
		rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out
}
