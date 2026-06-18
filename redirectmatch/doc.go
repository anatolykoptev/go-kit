// Package redirectmatch implements a pure, storage-free, two-tier redirect
// matching primitive.
//
// # Architecture
//
// Resolution is single-hop and first-match-wins:
//
//  1. Exact tier (O(1) hash lookup) — probed before the ordered tier.
//     Two separate maps prevent key-space collisions:
//     - exact: non-QExact Exact rules, keyed by Normalize(SourcePath, policy).
//     - exactQ: QExact Exact rules, keyed by Normalize(pathPart, policy) + "?" + rawQueryPart,
//     where pathPart and rawQueryPart are the halves of SourcePath split on the first "?".
//
//  2. Ordered tier (first-match prefix / RE2 scan) — rules sorted by
//     (Priority ASC, ID ASC). First match wins; no chaining.
//
// This is the source-of-truth specification, mirrored 1:1 by the piter-now
// TypeScript resolver. testdata/golden.json is the cross-language contract:
// every case in that file must pass in both Go and TypeScript.
//
// # QExact query-matching contract
//
// QExact matches the incoming query string by RAW byte-equality against the
// rawQueryPart embedded in SourcePath.  No normalization is applied to the
// query part.  In particular:
//   - Parameter order is significant: "?a=1&b=2" ≠ "?b=2&a=1".
//   - Case is significant: "?P=1" ≠ "?p=1".
//
// This is the deterministic contract the TypeScript mirror must replicate exactly.
//
// # QPass query-concatenation contract
//
// When QueryHandling is QPass, the rawQuery is appended to the resolved Location:
//   - rawQuery == ""               → append nothing.
//   - target has no "?"            → append "?" + rawQuery.
//   - target already contains "?"  → append "&" + rawQuery.
//
// rawQuery is assumed to be URL-clean (no fragment, caller strips "#…" if present).
// This byte-exact behavior is what the TypeScript mirror must replicate.
//
// # Residual regex self-loop gap
//
// [Compile] catches the static case: a Regex rule with no $n capture references
// in the Target that matches its own Target is rejected as a self-redirect.
// For Regex rules whose Target CONTAINS $n references, static analysis cannot
// prove whether the expansion will equal the input — that check is not attempted.
// The store-layer single-hop guarantee and an optional loop-guard at the store
// layer are the backstop for those cases.
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
