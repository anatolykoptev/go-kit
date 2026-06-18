// Package redirectmatch implements a pure, storage-free, two-tier redirect
// matching primitive.
//
// # Architecture
//
// Resolution is single-hop and first-match-wins:
//
//  1. Exact tier (O(1) hash lookup) — probed before the ordered tier.
//     A QExact rule is keyed by "path?query" so WordPress ?p=ID style
//     migrations work without touching the ordered scan.
//
//  2. Ordered tier (first-match prefix / RE2 scan) — rules sorted by
//     (Priority ASC, ID ASC). First match wins; no chaining.
//
// This is the source-of-truth specification, mirrored 1:1 by the piter-now
// TypeScript resolver. testdata/golden.json is the cross-language contract:
// every case in that file must pass in both Go and TypeScript.
//
// # Thread safety
//
// [RuleSet] is immutable after [BuildRuleSet] returns. [Resolve] is safe for
// concurrent use by multiple goroutines without synchronization.
//
// # Single-hop guarantee
//
// [Resolve] never re-resolves the produced Location against the set. Chains
// (A→B→C) are not followed: calling Resolve("/a") returns B, not C.
package redirectmatch
