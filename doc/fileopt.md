# fileopt

Byte-level lossless optimization for PDFs and images by shelling out to system
binaries (`gs` + `qpdf` for PDF, `oxipng` for PNG, `cwebp` for WebP). Missing
binaries are non-fatal — callers receive the original bytes plus a warn log.

```
go get github.com/anatolykoptev/go-kit/fileopt
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/fileopt"

raw, _ := os.ReadFile("report.pdf")

out, err := fileopt.OptimizeBytes(ctx, raw,
    fileopt.KindFromExt(filepath.Ext("report.pdf")),
    fileopt.LevelEbook, 80, // quality only applies to WebP
)
if err != nil {
    return err
}

log.Printf("ratio: %.2fx", fileopt.Ratio(len(raw), len(out)))
```

`KindFromExt` accepts `.pdf`, `.png`, `.webp` (with or without the leading
dot, any case). Anything else returns `KindUnsupported`; `OptimizeBytes` then
returns the input bytes unchanged.

## Guarantees

- **Lossless by default**. Every stage is guarded so the returned bytes are
  never larger than the input. If a stage would grow the file (e.g. `cwebp`
  on a smooth gradient), the original bytes pass through.
- **Content-aware routing**. Text-only PDFs skip the ghostscript stage,
  saving ~125ms of near-useless CPU while the `qpdf` structural pass still
  runs.
- **Missing-binary tolerant**. If `gs` / `qpdf` / `oxipng` / `cwebp` is
  missing on the host, the corresponding stage is skipped with a warn log;
  the call still succeeds with the input bytes.
- **Per-stage Prometheus metrics**. Operators see the marginal contribution
  of each subprocess independently of the combined reduction.

## PDF levels

`Level` selects the ghostscript PDF target profile:

| Level | DPI | Notes |
|-------|-----|-------|
| `LevelScreen` | 72  | smallest; readable but blurry |
| `LevelEbook`  | 150 | balanced default |
| `LevelPrinter` | 300 | higher quality |
| `LevelPrepress` | 300 | + colour-preserving |

```go
out, err := fileopt.OptimizePDF(ctx, raw, fileopt.LevelEbook)
```

## Per-format entrypoints

When you already know the format, call the typed function directly to avoid
`KindFromExt`:

```go
fileopt.OptimizePDF(ctx, data, fileopt.LevelEbook)
fileopt.OptimizePNG(ctx, data)                // oxipng, lossless
fileopt.OptimizeWebP(ctx, data, 80)            // cwebp at quality 80
```

## Metrics

Each subprocess records calls / duration / output-vs-input ratio / bytes
independently. Mount on your HTTP server:

```go
mux.Handle("/metrics", fileopt.MetricsHandler())
```

If you need to record from outside the package (test harnesses, custom
pipelines), `RecordSuccess(stage, bytesIn, bytesOut, dur)`, `RecordSkipped(stage)`,
and `RecordError(stage, dur)` are exposed.

## Environment overrides

Binary paths default to `$PATH` lookup. Override per-binary:

| Env var | Default name |
|---------|--------------|
| `FILEOPT_GS_PATH` | `gs` |
| `FILEOPT_QPDF_PATH` | `qpdf` |
| `FILEOPT_OXIPNG_PATH` | `oxipng` |
| `FILEOPT_CWEBP_PATH` | `cwebp` |

Useful on Alpine / minimal containers where the binaries live in a non-standard
prefix or are vendored alongside the service.

## API reference

| Symbol | Notes |
|--------|-------|
| `Kind` | enum: `KindUnsupported`, `KindPDF`, `KindPNG`, `KindWebP` |
| `KindFromExt(ext) Kind` | Maps file extension to Kind |
| `Level` | string enum for PDF target profile |
| `OptimizeBytes(ctx, data, kind, level, quality) ([]byte, error)` | One-call entrypoint |
| `OptimizePDF/PNG/WebP(...)` | Typed entrypoints |
| `Ratio(in, out) float64` | Convenience: `float64(in)/float64(out)` |
| `MetricsHandler() http.Handler` | Prometheus exposition |
| `RecordSuccess/Skipped/Error(...)` | External recording hooks |

## Notes

- All subprocesses receive the context — they're cancelled if `ctx` is. Make
  sure callers thread a deadline or the call may hang on a stuck binary.
- The optimizer writes intermediates to `os.TempDir()` and cleans them up;
  there is no on-disk artefact to manage.
- Concurrency is bounded by the OS process table, not the package. Use a
  [`ratelimit.ConcurrencyLimiter`](ratelimit.md) wrapper if you need a cap.
