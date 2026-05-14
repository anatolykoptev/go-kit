package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/httputil"
)

// Default CSP is go-nerv's stricter policy: script-src 'self' (no 'unsafe-inline').
// oxpulse-admin overrides via WithCSP to allow 'unsafe-inline'.

const (
	defaultCSP            = "default-src 'self'; script-src 'self'; style-src 'unsafe-inline'"
	adminCSP              = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
	defaultReferrerPolicy = "strict-origin-when-cross-origin"
)

// defaultHeaders is the canonical set of values SecurityHeaders writes.
// Used by assertHeaders to verify every header in one call.
var defaultHeaders = map[string]string{
	"X-Frame-Options":         "DENY",
	"X-Content-Type-Options":  "nosniff",
	"Referrer-Policy":         defaultReferrerPolicy,
	"Content-Security-Policy": defaultCSP,
	"Cache-Control":           "no-store",
}

// assertHeaders checks that every key in defaultHeaders is present with the
// expected value, applying overrides where supplied.
func assertHeaders(t *testing.T, h http.Header, overrides map[string]string) {
	t.Helper()
	for k, want := range defaultHeaders {
		if v, ok := overrides[k]; ok {
			want = v
		}
		if got := h.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestSecurityHeaders_Defaults(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec)
	assertHeaders(t, rec.Header(), nil)
}

func TestSecurityHeaders_WithCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec, httputil.WithCSP(adminCSP))
	assertHeaders(t, rec.Header(), map[string]string{
		"Content-Security-Policy": adminCSP,
	})
}

func TestSecurityHeaders_WithReferrerPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec, httputil.WithReferrerPolicy("no-referrer"))
	assertHeaders(t, rec.Header(), map[string]string{
		"Referrer-Policy": "no-referrer",
	})
}

// TestSecurityHeaders_ReplacesExistingHeader verifies that SecurityHeaders uses
// Set (not Add) semantics: a stale header value written by earlier middleware
// must be replaced, not appended.
func TestSecurityHeaders_ReplacesExistingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	// Pre-populate two headers with stale values that Add() would preserve.
	rec.Header().Set("Content-Security-Policy", "stale-value-from-earlier-middleware")
	rec.Header().Set("Referrer-Policy", "stale-referrer")

	httputil.SecurityHeaders(rec)

	// Content-Security-Policy must be replaced, not appended.
	cspVals := rec.Header().Values("Content-Security-Policy")
	if len(cspVals) != 1 {
		t.Fatalf("expected exactly 1 CSP value (Set semantics), got %d: %v", len(cspVals), cspVals)
	}
	if cspVals[0] == "stale-value-from-earlier-middleware" {
		t.Fatal("CSP not replaced — implementation likely uses Add() instead of Set()")
	}

	// Referrer-Policy must be replaced, not appended.
	rpVals := rec.Header().Values("Referrer-Policy")
	if len(rpVals) != 1 {
		t.Fatalf("expected exactly 1 Referrer-Policy value (Set semantics), got %d: %v", len(rpVals), rpVals)
	}
	if rpVals[0] == "stale-referrer" {
		t.Fatal("Referrer-Policy not replaced — implementation likely uses Add() instead of Set()")
	}
}
