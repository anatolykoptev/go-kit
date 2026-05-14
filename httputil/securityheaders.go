package httputil

import "net/http"

// Default security header values. The CSP default is the stricter go-nerv
// policy (script-src 'self', no 'unsafe-inline'). Callers that need a more
// permissive policy pass WithCSP explicitly.
const (
	defaultCSP            = "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'"
	defaultReferrerPolicy = "strict-origin-when-cross-origin"
)

// config holds overridable header values.
type config struct {
	csp            string
	referrerPolicy string
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

// SecurityHeaders writes a conservative set of HTTP security headers to w.
// Each header is set via Header().Set (replaces, never appends).
// Pass Option values to override specific headers from their defaults.
func SecurityHeaders(w http.ResponseWriter, opts ...Option) {
	cfg := config{
		csp:            defaultCSP,
		referrerPolicy: defaultReferrerPolicy,
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
}
