package httputil

import "net/http"

// Default security header values. The CSP default is the stricter go-nerv
// policy (script-src 'self', no 'unsafe-inline'). Callers that need a more
// permissive policy pass WithCSP explicitly.
//
// defaultPermissionsPolicy restricts camera, microphone, and geolocation —
// appropriate for admin and API UIs that don't need device access.
//
// defaultXSSProtection is "0" per OWASP and Mozilla Observatory: the legacy
// XSS auditor is disabled in modern browsers and is itself an XSS vector in
// older ones. Setting "0" explicitly ensures any residual browser behaviour
// is switched off.
const (
	defaultCSP               = "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'"
	defaultReferrerPolicy    = "strict-origin-when-cross-origin"
	defaultPermissionsPolicy = "camera=(), microphone=(), geolocation=()"
	defaultXSSProtection     = "0"
)

// config holds overridable header values.
type config struct {
	csp               string
	referrerPolicy    string
	permissionsPolicy string
	xssProtection     string
}

// Option modifies the header configuration applied by SecurityHeaders.
type Option func(*config)

// WithCSP overrides the Content-Security-Policy header value.
func WithCSP(policy string) Option {
	return func(c *config) { c.csp = policy }
}

// WithReferrerPolicy overrides the Referrer-Policy header value.
func WithReferrerPolicy(policy string) Option {
	return func(c *config) { c.referrerPolicy = policy }
}

// WithPermissionsPolicy overrides the Permissions-Policy header value.
// Default: "camera=(), microphone=(), geolocation=()" (no device access).
func WithPermissionsPolicy(policy string) Option {
	return func(c *config) { c.permissionsPolicy = policy }
}

// WithXSSProtection overrides the X-XSS-Protection header value.
// Default: "0" (disable legacy XSS auditor per OWASP/Mozilla Observatory).
func WithXSSProtection(value string) Option {
	return func(c *config) { c.xssProtection = value }
}

// SecurityHeaders writes a conservative set of HTTP security headers to w.
// Each header is set via Header().Set (replaces, never appends).
// Pass Option values to override specific headers from their defaults.
//
// Headers set by default:
//   - X-Content-Type-Options: nosniff
//   - X-Frame-Options: DENY
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Content-Security-Policy: default-src 'self'; script-src 'self'; ...
//   - Cache-Control: no-store
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//   - X-XSS-Protection: 0
//
// Downstream consumers (oxpulse-admin, go-nerv) will emit Permissions-Policy
// and X-XSS-Protection automatically on the next go-kit dependency bump —
// this is a deliberate widening of the security baseline.
func SecurityHeaders(w http.ResponseWriter, opts ...Option) {
	cfg := config{
		csp:               defaultCSP,
		referrerPolicy:    defaultReferrerPolicy,
		permissionsPolicy: defaultPermissionsPolicy,
		xssProtection:     defaultXSSProtection,
	}
	for _, o := range opts {
		o(&cfg)
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", cfg.referrerPolicy)
	h.Set("Content-Security-Policy", cfg.csp)
	h.Set("Cache-Control", "no-store")
	h.Set("Permissions-Policy", cfg.permissionsPolicy)
	h.Set("X-XSS-Protection", cfg.xssProtection)
}
