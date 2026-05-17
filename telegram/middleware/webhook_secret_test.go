package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// nextCalled is a sentinel handler that records whether ServeHTTP was invoked.
type nextCalled struct {
	called bool
}

func (n *nextCalled) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.called = true
	w.WriteHeader(http.StatusOK)
}

// TestWebhookSecretToken_ValidHeader_PassesThrough verifies that a matching
// secret token header results in the next handler being invoked with 200.
func TestWebhookSecretToken_ValidHeader_PassesThrough(t *testing.T) {
	secret := "my-secret-token"
	next := &nextCalled{}
	h := WebhookSecretToken(secret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if !next.called {
		t.Error("next handler was not called for valid secret token")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// TestWebhookSecretToken_InvalidHeader_Returns401 verifies that an incorrect
// header value → 401 and next handler is NOT called.
func TestWebhookSecretToken_InvalidHeader_Returns401(t *testing.T) {
	secret := "correct-secret"
	next := &nextCalled{}
	h := WebhookSecretToken(secret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if next.called {
		t.Error("next handler must NOT be called for invalid secret token")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestWebhookSecretToken_MissingHeader_Returns401 verifies that a missing
// X-Telegram-Bot-Api-Secret-Token header → 401.
func TestWebhookSecretToken_MissingHeader_Returns401(t *testing.T) {
	secret := "my-secret"
	next := &nextCalled{}
	h := WebhookSecretToken(secret)(next)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	// No header set.
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if next.called {
		t.Error("next handler must NOT be called when header is absent")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestWebhookSecretToken_DifferentLengthHeaders_StillSafe verifies that
// mismatched-length tokens still result in 401. crypto/subtle.ConstantTimeCompare
// handles length differences without short-circuit so there is no early-exit
// timing leak on length (unlike naive bytes.Equal). We cannot assert timing here
// in a unit test, but we assert the functional correctness.
func TestWebhookSecretToken_DifferentLengthHeaders_StillSafe(t *testing.T) {
	secret := "long-secret-token-value"
	next := &nextCalled{}
	h := WebhookSecretToken(secret)(next)

	shortHeader := "short"
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", shortHeader)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if next.called {
		t.Error("next handler must NOT be called when header length differs from secret")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestWebhookSecretToken_EmptySecret_RejectsEmptyHeader verifies defense-in-depth:
// even when secret="" an empty header must be rejected (both empty → match is
// forbidden as a misconfiguration guard). Documents the deliberate behaviour.
//
// Note: ConstantTimeCompare([]byte(""), []byte("")) == 1, so an empty secret
// would match an empty header. This test documents that the implementation MUST
// treat an empty secret as always-reject. Callers should never configure an
// empty secret, but the middleware must not silently accept any request when
// misconfigured.
func TestWebhookSecretToken_EmptySecret_RejectsEmptyHeader(t *testing.T) {
	// Per Telegram docs, secret_token must be 1-256 chars. Empty is invalid.
	// Middleware should defensively reject all requests when secret is empty.
	next := &nextCalled{}
	h := WebhookSecretToken("")(next)

	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	// No header → header value is also "".
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if next.called {
		t.Error("next handler must NOT be called when secret is empty (misconfiguration guard)")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
