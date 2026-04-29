package rerank

// linearMinMaxFlatScore is the per-list contribution applied when a list has
// max == min (all identical scores, or a single item). 0.5 is chosen over 0
// because the list DID retrieve the doc — treating it as "midpoint relevant"
// is closer to operator intent than "irrelevant", and consistent with how
// Elasticsearch's Linear Retriever documents the degenerate case.
const linearMinMaxFlatScore = 0.5

// LinearMinMax fuses N score-bearing lists by MinMax-normalizing each list's
// scores into [0, 1] and summing per ID with caller-supplied weights:
//
//	score(d) = Σ w_i * (raw_i(d) - min_i) / (max_i - min_i)
//
// This is the Elasticsearch Linear Retriever convention. Use with calibrated
// weights (e.g. from grid search on a held-out dev set) when you want
// transparent, easily-debugged fused scores in the [0, Σw] range.
//
// Edge cases:
//   - max == min in a list → all entries normalize to linearMinMaxFlatScore
//     (= 0.5). The list cannot rank its own results, so it contributes a
//     uniform "list saw this doc" signal. Weight still applies.
//   - Empty list → contributes nothing.
//   - Single-item list → falls into the max==min branch above (still gets
//     the flat 0.5 weighted contribution).
//   - weight=0 → list skipped entirely (no normalization performed; saves work).
//   - Negative weight → allowed as a penalty term, identical handling to
//     WeightedRRF.
//   - Duplicate IDs inside one list: only the FIRST occurrence in that list
//     contributes (mirrors RRF/DBSF "best first" rule).
//
// Tie-breaking: stable, first-seen order across lists.
//
// Panics if len(weights) != len(lists). This is a programmer error: weights
// and lists are nearly always specified together at config-parse time.
func LinearMinMax(weights []float64, lists ...ScoredIDList) []Fused {
	if len(weights) != len(lists) {
		panic("rerank.LinearMinMax: len(weights) != len(lists)")
	}

	scores := make(map[string]float64)
	order := make([]string, 0)

	for li, list := range lists {
		linearMinMaxAccumulateList(weights[li], list, scores, &order)
	}

	recordLinearMinMaxListsFused(len(lists))
	return sortFused(scores, order)
}

// linearMinMaxAccumulateList MinMax-normalizes one list and accumulates the
// weighted contribution into scores. weight=0 or empty list are no-ops.
func linearMinMaxAccumulateList(w float64, list ScoredIDList, scores map[string]float64, order *[]string) {
	if w == 0 || len(list) == 0 {
		return
	}
	dedup := dedupByFirstID(list)
	minS, maxS := minMaxScore(dedup)
	span := maxS - minS
	for _, item := range dedup {
		if _, ok := scores[item.ID]; !ok {
			*order = append(*order, item.ID)
		}
		norm := linearMinMaxFlatScore
		if span != 0 {
			norm = (item.Score - minS) / span
		}
		scores[item.ID] += w * norm
	}
}

// minMaxScore returns the min and max Score across list. Caller guarantees
// len(list) > 0.
func minMaxScore(list ScoredIDList) (minS, maxS float64) {
	minS, maxS = list[0].Score, list[0].Score
	for _, item := range list[1:] {
		if item.Score < minS {
			minS = item.Score
		}
		if item.Score > maxS {
			maxS = item.Score
		}
	}
	return minS, maxS
}
