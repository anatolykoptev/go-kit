# uploads

Canonical filesystem layout for files services produce on the local box:
screenshots, generated images, PDFs, audit reports — anything that needs to
live somewhere predictable.

```
go get github.com/anatolykoptev/go-kit/uploads
```

## Convention

```
$UPLOADS_ROOT/<service>/<bucket>/<filename>
```

| Component | Notes |
|-----------|-------|
| `$UPLOADS_ROOT` | Default `$HOME/uploads`; override via env (`UPLOADS_ROOT`) |
| `<service>` | Producing service name (`go-wowa`, `go-imagine`, `vaelor`) |
| `<bucket>` | Producer's internal grouping (`screenshots`, `carousels`, `pdf`, `audio`) |
| `<filename>` | Actual filename — passed through as-is, no sanitisation |

## Quick start

```go
import "github.com/anatolykoptev/go-kit/uploads"

// One-shot path; parent dirs created.
path, err := uploads.Path("vaelor", "imagined", "sf-rooftop.png")
if err != nil {
    return err
}
err = os.WriteFile(path, pngBytes, 0o644)
```

When the service wants to manage its own internal layout below the bucket:

```go
dir, err := uploads.Bucket("go-wowa", "screenshots")
// dir = "$UPLOADS_ROOT/go-wowa/screenshots", created if missing
out := filepath.Join(dir, time.Now().Format("2006/01/02"), "shot.png")
```

For service-wide setup that owns its own subtree:

```go
dir, err := uploads.Service("go-imagine") // "$UPLOADS_ROOT/go-imagine"
```

## API reference

| Function | Notes |
|----------|-------|
| `Root() string` | Read-only resolution: `$UPLOADS_ROOT` → `$HOME/uploads` → `$TMPDIR/uploads`. Does NOT mkdir |
| `Service(name) (string, error)` | `Root/<name>`; `MkdirAll(0755)`; errors on empty name |
| `Bucket(service, bucket) (string, error)` | `Root/<service>/<bucket>`; mkdir; empty bucket falls back to `Service` |
| `Path(service, bucket, filename) (string, error)` | `Bucket(...) + /filename`; parent dirs created, filename untouched |
| `EnvRoot` const | `"UPLOADS_ROOT"` — the env var name |
| `DefaultRootRel` const | `"uploads"` — joined onto `$HOME` when env is unset |

## Why a shared package

Every service was rolling its own `/tmp/<svc>-*` path pattern. Hard to find
files, no shared retention policy, no operator overview. With this convention,
`cd ~/uploads && ls` shows every service's recent output at a glance, and a
single cron / systemd timer can rotate stale entries across the whole box.

## Notes

- `Path` does NOT sanitise the filename — callers MUST validate it (no
  `../`, no absolute paths, length limits) before passing user-controlled
  input. See `path.Clean` and the `safePathJoin` pattern in the security
  guidelines.
- All `Mkdir` calls use mode `0o755`. The umask may further restrict; do not
  rely on world-readable output on hardened hosts.
- `Root()` falls back to `$TMPDIR/uploads` only as a last resort
  (`os.UserHomeDir` failed). In that case, files do not survive a reboot —
  log a warn at startup if you care.
- Set `UPLOADS_ROOT=/var/lib/uploads` in production to escape `$HOME`'s
  quotas / backup policies.
