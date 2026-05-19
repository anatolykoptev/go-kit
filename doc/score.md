# score

Tiny helpers for translating numeric scores into ordinal labels — confidence
levels, severity tiers, custom buckets.

```
go get github.com/anatolykoptev/go-kit/score
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/score"

c := score.ConfidenceFromScore(0.82) // -> score.ConfidenceHigh

if score.SeverityAtLeast(found, score.SeverityHigh) {
    pageOncall()
}

label := score.Bucket(0.42,
    []float64{0.2, 0.5, 0.8},
    []string{"low", "medium", "high", "extreme"},
) // -> "medium"
```

## Confidence levels

`ConfidenceFromScore(s)` maps `[0,1]` to `ConfidenceLow` / `ConfidenceMedium`
/ `ConfidenceHigh` using default thresholds (`< 0.33`, `< 0.66`, else high).
`ConfidenceFromScoreThresholds(s, lowMax, mediumMax)` lets the caller pick
custom cut-points.

## Severity

A small ordinal enum (`info` < `low` < `medium` < `high` < `critical`) with
parsing and rank helpers:

| Function | Notes |
|----------|-------|
| `ParseSeverity(s)` | Case-insensitive; returns `("", false)` on unknown |
| `SeverityRank(s)` | int rank, useful for `<` comparisons in sorts |
| `SeverityAtLeast(s, threshold)` | `true` when `s >= threshold` |
| `SeverityFromScore(s)` | `[0,1]` → severity using default cut-points |
| `Severity.String()` | Lower-case canonical form |

## Bucket — arbitrary ordinal labels

`Bucket(s, thresholds, labels)` returns the `labels[i]` for which `s` falls
into the `i`-th bucket (boundaries are *upper-exclusive*). `len(labels)` must
equal `len(thresholds) + 1`.

```go
score.Bucket(rate, []float64{0.5, 0.9, 0.99}, []string{"green","yellow","orange","red"})
```

Use when neither confidence nor severity labels fit your domain (e.g. SLO
burn-rate buckets, p95 latency tiers).

## API reference

| Symbol | Notes |
|--------|-------|
| `ConfidenceLevel` | string enum: `ConfidenceLow`, `ConfidenceMedium`, `ConfidenceHigh` |
| `ConfidenceFromScore(s float64) ConfidenceLevel` | default thresholds 0.33 / 0.66 |
| `ConfidenceFromScoreThresholds(s, lowMax, mediumMax float64) ConfidenceLevel` | custom thresholds |
| `Severity` | string enum: `SeverityInfo`, `SeverityLow`, `SeverityMedium`, `SeverityHigh`, `SeverityCritical` |
| `ParseSeverity(s string) (Severity, bool)` | parse from any case |
| `SeverityRank(s Severity) int` | for sorting / `<` comparisons |
| `SeverityFromScore(s float64) Severity` | `[0,1]` → severity tier |
| `SeverityAtLeast(s, threshold Severity) bool` | inclusive comparison |
| `Bucket(s float64, thresholds []float64, labels []string) string` | generic ordinal mapping |

## When to use which

| You have | Want | Reach for |
|----------|------|-----------|
| ML confidence `[0,1]` | "low/med/high" tag | `ConfidenceFromScore` |
| Security finding or alert score | severity tier for routing | `SeverityFromScore` + `SeverityAtLeast` |
| Domain-specific scale | named buckets you control | `Bucket` |

No errors, no allocations, pure functions — embed wherever scoring needs a
human-readable label.
