package httputil

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// ClientIP extracts the real client address. Services behind a reverse proxy
// (e.g. Caddy) typically see only the loopback address in r.RemoteAddr.
//
// Priority:
//  1. X-Real-IP — set by the proxy to the single trusted client IP; most authoritative.
//  2. X-Forwarded-For first element — fallback for proxies that don't set X-Real-IP.
//  3. r.RemoteAddr — last resort (always loopback behind a proxy, but correct in tests).
//
// Each candidate is validated via net.ParseIP before use. An invalid value
// triggers a WARN log and falls through to the next candidate. This prevents
// key-space pollution and log injection from forged headers.
func ClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		candidate := strings.TrimSpace(xri)
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
		slog.Warn("ClientIP: invalid X-Real-IP header, falling through",
			"value", candidate,
			"remote_addr", r.RemoteAddr,
		)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		candidate := strings.TrimSpace(parts[0])
		if ip := net.ParseIP(candidate); ip != nil {
			return ip.String()
		}
		slog.Warn("ClientIP: invalid X-Forwarded-For first element, falling through",
			"value", candidate,
			"remote_addr", r.RemoteAddr,
		)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
