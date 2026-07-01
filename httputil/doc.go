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
//
// # SSRF Guard
//
// This is the single, framework-owned SSRF block-list for any go-kit
// service that fetches a caller-supplied URL (an image src, an advertiser
// website, a redirect Location header, an out-of-process render delegate's
// target). It supersedes the two guards it consolidates -- render/html's
// former unexported isPrivateIP/safeDial and go-enriche's fetch/ssrf.go --
// and adds the ranges neither one covered: CGNAT (100.64.0.0/10), the NAT64
// well-known prefix (64:ff9b::/96), 6to4 (2002::/16), and the deprecated
// IPv4-compatible IPv6 form (::/96).
//
// IsBlockedIP is the low-level predicate: loopback, RFC1918/RFC4193
// private, link-local (incl. the 169.254.169.254 cloud-metadata address),
// unspecified, multicast, plus the four ranges above. Every other function
// here is built on top of it.
//
// GuardedDialContext wraps a *net.Dialer with a Control hook that checks
// the ALREADY-RESOLVED address at connect time, immediately before the
// connect(2) syscall -- this defeats DNS-rebinding, since the check
// inspects the literal address about to be dialed, never a hostname string
// that could resolve differently between lookup and connect.
//
// NewSSRFGuardedClient wraps an *http.Client in two tiers depending on its
// Transport: a *http.Transport (or nil) gets DialContext replaced with
// GuardedDialContext -- the strong, connect-time tier; any other
// http.RoundTripper (e.g. a stealth/fingerprint-evasion client with no
// exposed dial hook) gets a pre-request CheckURL check instead -- a
// necessarily weaker, pre-resolve tier, but the best available without
// reaching into a dial mechanism this package does not own.
//
// CheckURL is the pre-handoff check for a URL this package never dials
// itself -- e.g. before handing an "any place" URL to an out-of-process
// render delegate. It enforces an http/https scheme allowlist (a headless
// browser coerced into file:// or gopher:// bypasses every IP-based check,
// since those schemes never dial the checked host at all) plus IsBlockedIP
// on every resolved address. Call it again after each redirect hop a
// delegate reports, to minimize the DNS-rebind window. A host that looks
// like a non-standard IP encoding (decimal, octal, or hex -- e.g.
// "2130706433" or "0x7f000001") but fails net.ParseIP is refused outright
// rather than handed to DNS resolution, since some resolvers still parse
// those forms as literal IPs.
package httputil
