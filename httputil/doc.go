// Package httputil provides small HTTP helpers shared across go-kit services.
//
// # Security Headers
//
// SecurityHeaders sets a conservative set of HTTP security headers on a
// ResponseWriter. Defaults follow go-nerv stricter policy:
//
//   - Content-Security-Policy: default-src self; script-src self; style-src self unsafe-inline
//   - X-Content-Type-Options: nosniff
//   - X-Frame-Options: DENY
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//   - X-XSS-Protection: 0
//
// Cache-Control is NOT set by default. Cache policy is orthogonal to security
// headers — marketing pages, API endpoints, and authed admin pages have
// fundamentally different caching requirements. Use WithCacheControl to set it:
//
//	SecurityHeaders(w, WithCacheControl("no-store"))           // authed admin
//	SecurityHeaders(w, WithCacheControl("public, max-age=3600")) // public assets
//
// Services that require a less restrictive CSP (e.g. oxpulse-admin, which
// needs unsafe-inline for its embedded scripts) should call
// SecurityHeaders(w, WithCSP("...")) to override only that header while
// keeping all other defaults.
//
// # Client IP
//
// ClientIP extracts the real client address from an HTTP request. Consults
// X-Real-IP, then X-Forwarded-For (first hop), then r.RemoteAddr. Each
// candidate is validated via net.ParseIP -- invalid or spoofed header values
// are silently skipped rather than returned. See ClientIP for the full trust
// model and deployment requirements.
package httputil
