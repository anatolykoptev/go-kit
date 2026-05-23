package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anatolykoptev/go-kit/httputil"
)

// Default CSP includes 'self' in style-src so own /static/*.css loads.
// oxpulse-admin overrides via WithCSP to also allow inline scripts.

const (
	defaultCSP               = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'"
	adminCSP                 = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
	defaultReferrerPolicy    = "strict-origin-when-cross-origin"
	defaultPermissionsPolicy = "camera=(), microphone=(), geolocation=()"
	defaultXSSProtection     = "0"
)

// defaultHeaders is the canonical set of values SecurityHeaders writes.
// Used by assertHeaders to verify every header in one call.
// Cache-Control is intentionally absent: SecurityHeaders does not set it by
// default. Callers must use WithCacheControl or set it directly.
var defaultHeaders = map[string]string{
	"X-Frame-Options":         "DENY",
	"X-Content-Type-Options":  "nosniff",
	"Referrer-Policy":         defaultReferrerPolicy,
	"Content-Security-Policy": defaultCSP,
	"Permissions-Policy":      defaultPermissionsPolicy,
	"X-XSS-Protection":        defaultXSSProtection,
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

// ---------------------------------------------------------------------------
// Commit 3: Permissions-Policy and X-XSS-Protection defaults
// ---------------------------------------------------------------------------

// TestSecurityHeaders_DefaultsIncludePermissionsPolicy verifies the new default.
func TestSecurityHeaders_DefaultsIncludePermissionsPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec)
	if got := rec.Header().Get("Permissions-Policy"); got != defaultPermissionsPolicy {
		t.Errorf("Permissions-Policy = %q, want %q", got, defaultPermissionsPolicy)
	}
}

// TestSecurityHeaders_DefaultsIncludeXSSProtectionZero verifies the new default.
func TestSecurityHeaders_DefaultsIncludeXSSProtectionZero(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec)
	if got := rec.Header().Get("X-XSS-Protection"); got != "0" {
		t.Errorf("X-XSS-Protection = %q, want %q", got, "0")
	}
}

// TestSecurityHeaders_WithPermissionsPolicy verifies the override option.
func TestSecurityHeaders_WithPermissionsPolicy(t *testing.T) {
	rec := httptest.NewRecorder()
	custom := "camera=(), microphone=()"
	httputil.SecurityHeaders(rec, httputil.WithPermissionsPolicy(custom))
	assertHeaders(t, rec.Header(), map[string]string{
		"Permissions-Policy": custom,
	})
}

// TestSecurityHeaders_WithXSSProtection verifies the override option.
func TestSecurityHeaders_WithXSSProtection(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec, httputil.WithXSSProtection("1; mode=block"))
	assertHeaders(t, rec.Header(), map[string]string{
		"X-XSS-Protection": "1; mode=block",
	})
}

// ---------------------------------------------------------------------------
// Cache-Control behaviour (BREAKING CHANGE v0.56.0)
// ---------------------------------------------------------------------------

// TestSecurityHeadersDoesNotSetCacheControlByDefault verifies that
// SecurityHeaders no longer sets Cache-Control unconditionally.
// Callers must declare cache policy explicitly — marketing pages and admin
// pages have different requirements and the function must not guess.
func TestSecurityHeadersDoesNotSetCacheControlByDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.SecurityHeaders(rec)
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control = %q, want empty (not set by default)", got)
	}
}

// TestWithCacheControlExplicit verifies that WithCacheControl sets the header
// when the caller opts in — both no-store (authed admin) and public (assets).
func TestWithCacheControlExplicit(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"no-store for authed admin", "no-store"},
		{"public max-age for marketing page", "public, max-age=3600"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			httputil.SecurityHeaders(rec, httputil.WithCacheControl(tc.value))
			if got := rec.Header().Get("Cache-Control"); got != tc.value {
				t.Errorf("Cache-Control = %q, want %q", got, tc.value)
			}
		})
	}
}
