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

// eligibleEndpoints filters eps to those not currently in cooldown.
// This is Guard A of the cooled-model exclusion invariant: it builds the
// non-cooled subset before shuffling, so a cooled model is never placed into
// the try-order at all. Guard B (the per-ep cooling() check in the loop body
// of executeInner) is a race-safety backstop for the concurrent-cooldown
// window; it does not cover this point-in-time filtering.
// Called only when skipCooled=true (≥1 healthy endpoint exists) and strategy
// is SelectionRandom.
func eligibleEndpoints(all []Endpoint, cd *modelCooldown) []Endpoint {
	out := make([]Endpoint, 0, len(all))
	for _, ep := range all {
		if !cd.cooling(ep.Model) {
			out = append(out, ep)
		}
	}
	return out
}
