package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
)

// HTTPMiddleware wraps an http.Handler, adding cross-cutting HTTP-layer behaviour.
// It is a distinct type from [Middleware], which operates on Telegram Updates.
// HTTPMiddleware sits in front of the webhook HTTP handler — before any Update
// is parsed — so it is appropriate for transport-layer gates such as secret-token
// validation.
type HTTPMiddleware func(http.Handler) http.Handler

// WebhookSecretToken returns an [HTTPMiddleware] that validates the
// X-Telegram-Bot-Api-Secret-Token header on every incoming request.
//
// Telegram sends this header when a secret_token is configured via setWebhook.
// The comparison uses [crypto/subtle.ConstantTimeCompare] to prevent timing
// attacks — a vulnerability present in the majority of competitor implementations
// that use plain string equality or bytes.Equal (which short-circuit on the first
// differing byte, leaking information about how many bytes matched).
//
// If the header is absent, empty, or mismatched, the middleware responds with
// 401 Unauthorized and logs a warning via [slog]. The next handler is not called.
//
// Defense-in-depth: an empty secret always rejects all requests, including those
// with an empty header. Per Telegram Bot API docs the secret_token must be
// 1–256 characters [A-Za-z0-9_-]; an empty value signals misconfiguration.
//
// Recommended position: outermost HTTP middleware, before any body parsing.
//
//	mux.Handle("/webhook", middleware.WebhookSecretToken(secret)(updateHandler))
func WebhookSecretToken(secret string) HTTPMiddleware {
	secretB := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")

			// Empty secret is a misconfiguration — reject unconditionally so a
			// bot with no secret configured cannot be spoofed by sending an
			// empty header. This check must come before ConstantTimeCompare
			// because ConstantTimeCompare([]byte(""), []byte("")) == 1.
			if len(secret) == 0 || subtle.ConstantTimeCompare([]byte(hdr), secretB) != 1 {
				slog.WarnContext(r.Context(), "webhook secret token mismatch",
					slog.String("remote_addr", r.RemoteAddr),
					slog.Int("hdr_len", len(hdr)))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
