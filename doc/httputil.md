# httputil

Tiny HTTP helpers shared across go-kit services: real client IP extraction
behind reverse proxies and a one-call security-headers setter with sane defaults.

```
go get github.com/anatolykoptev/go-kit/httputil
```

## Quick start

```go
import "github.com/anatolykoptev/go-kit/httputil"

func handler(w http.ResponseWriter, r *http.Request) {
    httputil.SecurityHeaders(w)

    ip := httputil.ClientIP(r)
    slog.Info("request", "ip", ip, "path", r.URL.Path)

    w.Write([]byte("ok"))
}
```

## ClientIP — real-IP behind a proxy

Walks (in order) `X-Real-IP`, `X-Forwarded-For` (first hop), then
`r.RemoteAddr`. Each candidate is validated via `net.ParseIP` — invalid /
spoofed values are silently skipped, not returned. Returns the first valid
address as a bare IP string.

```go
ip := httputil.ClientIP(r)
```

**Trust model**: ClientIP trusts the *first* `X-Forwarded-For` hop. That is
correct only when your reverse proxy strips and re-writes the header itself.
If the proxy chain is `client → CDN → LB → app`, `X-Forwarded-For: client,
CDN, LB` is what `r.Header` contains, and ClientIP returns `client` — the
expected behaviour. If the proxy passes through caller-supplied
`X-Forwarded-For`, ClientIP returns whatever the attacker wants. Always pair
with a proxy that overwrites these headers.

## SecurityHeaders — one-call defaults

Sets a conservative HTTP security header set on the `ResponseWriter`:

| Header | Default |
|--------|---------|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'unsafe-inline'` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` |
| `X-XSS-Protection` | `0` |

`Cache-Control` is intentionally NOT set by default — caching policy is
orthogonal to security headers (marketing pages, API endpoints, admin pages
have different needs).

## Overriding defaults

Pass options to relax / tighten individual headers without losing the others:

```go
httputil.SecurityHeaders(w,
    httputil.WithCSP("default-src 'self'; script-src 'self' 'unsafe-inline'"),
    httputil.WithCacheControl("no-store"),
)
```

| Option | Header it sets |
|--------|----------------|
| `WithCSP(policy)` | `Content-Security-Policy` |
| `WithReferrerPolicy(policy)` | `Referrer-Policy` |
| `WithPermissionsPolicy(policy)` | `Permissions-Policy` |
| `WithXSSProtection(value)` | `X-XSS-Protection` |
| `WithCacheControl(value)` | `Cache-Control` (otherwise unset) |

## When to call

`SecurityHeaders` mutates response headers — call it *before* the first
`w.Write` / `w.WriteHeader`, ideally in a middleware:

```go
func secMW(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        httputil.SecurityHeaders(w)
        next.ServeHTTP(w, r)
    })
}
```

## Notes

- This package is intentionally tiny — observability middleware lives in
  [`metrics/httpmw`](metrics.md), tracing in
  [`tracing/httpmw`](tracing.md). httputil is for headers + IP only.
- `SecurityHeaders` does not validate option values — pass syntactically valid
  CSP / Permissions-Policy strings.
- If your service has *less* restrictive needs (e.g. an admin panel using
  `unsafe-inline`), set `WithCSP("...")` rather than skipping
  `SecurityHeaders` altogether — you still want every other default header.
