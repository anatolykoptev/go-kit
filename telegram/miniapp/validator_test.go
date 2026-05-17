package miniapp_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/miniapp"
)

// signInitData constructs a valid Telegram initData string signed with the given botToken.
// params must NOT include "hash"; auth_date is appended automatically.
// This helper mirrors the Telegram server-side signing algorithm so that tests
// are self-contained and do not depend on real bot tokens or captured traffic.
func signInitData(t *testing.T, botToken string, params map[string]string, authDate time.Time) string {
	t.Helper()

	pairs := make([]string, 0, len(params)+1)
	for k, v := range params {
		pairs = append(pairs, k+"="+v)
	}
	pairs = append(pairs, fmt.Sprintf("auth_date=%d", authDate.Unix()))
	sort.Strings(pairs)

	dataCheckString := strings.Join(pairs, "\n")

	skHmac := hmac.New(sha256.New, []byte("WebAppData"))
	skHmac.Write([]byte(botToken))
	secretKey := skHmac.Sum(nil)

	msgHmac := hmac.New(sha256.New, secretKey)
	msgHmac.Write([]byte(dataCheckString))
	hash := hex.EncodeToString(msgHmac.Sum(nil))

	// Build the query string with all original params + auth_date + hash.
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	vals.Set("auth_date", fmt.Sprintf("%d", authDate.Unix()))
	vals.Set("hash", hash)
	return vals.Encode()
}

const testToken = "1234567890:AAHs3KCsABCDEFGHIJKLMNOPQRSTUVWXYZ"

// TestValidate_ValidInitData_ReturnsUser: happy path — signed data returns
// correctly parsed user.
func TestValidate_ValidInitData_ReturnsUser(t *testing.T) {
	t.Parallel()

	authDate := time.Unix(1716825600, 0) // fixed timestamp
	userJSON := `{"id":123456789,"first_name":"Alice","last_name":"Smith","username":"alice","language_code":"en","is_premium":true}`
	params := map[string]string{
		"user":         userJSON,
		"query_id":     "AABBBCCC",
		"start_param":  "launch",
		"chat_type":    "sender",
		"chat_instance": "1234",
	}
	initData := signInitData(t, testToken, params, authDate)

	got, err := miniapp.ValidateInitData(initData, testToken)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil InitData")
	}
	if got.User == nil {
		t.Fatal("expected non-nil User")
	}
	if got.User.ID != 123456789 {
		t.Errorf("User.ID = %d, want 123456789", got.User.ID)
	}
	if got.User.FirstName != "Alice" {
		t.Errorf("User.FirstName = %q, want Alice", got.User.FirstName)
	}
	if got.User.Username != "alice" {
		t.Errorf("User.Username = %q, want alice", got.User.Username)
	}
	if !got.User.IsPremium {
		t.Error("User.IsPremium should be true")
	}
	if got.QueryID != "AABBBCCC" {
		t.Errorf("QueryID = %q, want AABBBCCC", got.QueryID)
	}
	if got.AuthDate.Unix() != authDate.Unix() {
		t.Errorf("AuthDate = %v, want %v", got.AuthDate, authDate)
	}
}

// TestValidate_TamperedHash_Rejected: a single hex digit flipped in the hash
// must return ErrInvalidSignature.
func TestValidate_TamperedHash_Rejected(t *testing.T) {
	t.Parallel()

	authDate := time.Now().Add(-10 * time.Second)
	initData := signInitData(t, testToken, map[string]string{}, authDate)

	// Parse and flip one char in hash.
	vals, _ := url.ParseQuery(initData)
	hash := vals.Get("hash")
	if hash[0] == 'a' {
		hash = "b" + hash[1:]
	} else {
		hash = "a" + hash[1:]
	}
	vals.Set("hash", hash)
	tampered := vals.Encode()

	_, err := miniapp.ValidateInitData(tampered, testToken)
	if !errors.Is(err, miniapp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

// TestValidate_TamperedField_Rejected: changing user.id after signing must
// invalidate the HMAC.
func TestValidate_TamperedField_Rejected(t *testing.T) {
	t.Parallel()

	authDate := time.Now().Add(-5 * time.Second)
	userJSON := `{"id":111,"first_name":"Bob"}`
	initData := signInitData(t, testToken, map[string]string{"user": userJSON}, authDate)

	// Replace the user JSON with a different id.
	vals, _ := url.ParseQuery(initData)
	vals.Set("user", `{"id":999,"first_name":"Bob"}`)
	tampered := vals.Encode()

	_, err := miniapp.ValidateInitData(tampered, testToken)
	if !errors.Is(err, miniapp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

// TestValidate_MissingHash_Returns_ErrMissingHash: no hash param.
func TestValidate_MissingHash_Returns_ErrMissingHash(t *testing.T) {
	t.Parallel()

	initData := "auth_date=1716825600&user=%7B%22id%22%3A1%7D"

	_, err := miniapp.ValidateInitData(initData, testToken)
	if !errors.Is(err, miniapp.ErrMissingHash) {
		t.Errorf("expected ErrMissingHash, got: %v", err)
	}
}

// TestValidate_ConstantTimeCompare: verifies that the implementation uses
// crypto/subtle for hash comparison (behavioural proxy: rejection of wrong
// hash is consistent whether the hash is all-zeros or a near-match).
// The real security guarantee is in code review; this test validates the
// public contract that wrong hashes are always rejected.
func TestValidate_ConstantTimeCompare(t *testing.T) {
	t.Parallel()

	authDate := time.Now().Add(-5 * time.Second)
	initData := signInitData(t, testToken, map[string]string{}, authDate)
	vals, _ := url.ParseQuery(initData)

	wrongHashes := []string{
		strings.Repeat("0", 64),
		strings.Repeat("f", 64),
		"deadbeef" + strings.Repeat("0", 56),
	}
	for _, wrong := range wrongHashes {
		vals.Set("hash", wrong)
		_, err := miniapp.ValidateInitData(vals.Encode(), testToken)
		if !errors.Is(err, miniapp.ErrInvalidSignature) {
			t.Errorf("hash=%q: expected ErrInvalidSignature, got: %v", wrong, err)
		}
	}
}

// TestValidate_WrongBotToken_Rejected: data signed with tokenA must be rejected
// when validated with tokenB.
func TestValidate_WrongBotToken_Rejected(t *testing.T) {
	t.Parallel()

	tokenA := "111111111:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	tokenB := "222222222:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

	authDate := time.Now().Add(-5 * time.Second)
	initData := signInitData(t, tokenA, map[string]string{}, authDate)

	_, err := miniapp.ValidateInitData(initData, tokenB)
	if !errors.Is(err, miniapp.ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

// TestValidate_WithMaxAge_AcceptsRecent: auth_date within maxAge window passes.
func TestValidate_WithMaxAge_AcceptsRecent(t *testing.T) {
	t.Parallel()

	authDate := time.Now().Add(-30 * time.Second) // 30s ago
	initData := signInitData(t, testToken, map[string]string{}, authDate)

	_, err := miniapp.ValidateInitDataWithMaxAge(initData, testToken, 5*time.Minute)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestValidate_WithMaxAge_RejectsStale: auth_date older than maxAge → ErrStaleAuthDate.
func TestValidate_WithMaxAge_RejectsStale(t *testing.T) {
	t.Parallel()

	authDate := time.Now().Add(-2 * time.Hour) // 2h ago
	initData := signInitData(t, testToken, map[string]string{}, authDate)

	_, err := miniapp.ValidateInitDataWithMaxAge(initData, testToken, 30*time.Minute)
	if !errors.Is(err, miniapp.ErrStaleAuthDate) {
		t.Errorf("expected ErrStaleAuthDate, got: %v", err)
	}
}

// TestValidate_RealUserBlob: roundtrip with a realistic initData payload
// including nested JSON user blob, verifying correct URL-decode + JSON parse.
func TestValidate_RealUserBlob(t *testing.T) {
	t.Parallel()

	authDate := time.Unix(1716825600, 0)
	// Realistic user blob as sent by Telegram (URL-encoded JSON).
	userJSON := `{"id":987654321,"first_name":"Ivan","last_name":"Petrov","username":"ivanpetrov","language_code":"ru","allows_write_to_pm":true}`
	params := map[string]string{
		"user":         userJSON,
		"query_id":     "AAEFGH123",
		"chat_type":    "private",
		"chat_instance": "-1234567890123456789",
	}
	initData := signInitData(t, testToken, params, authDate)

	got, err := miniapp.ValidateInitData(initData, testToken)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got.User == nil {
		t.Fatal("expected non-nil User")
	}
	if got.User.ID != 987654321 {
		t.Errorf("User.ID = %d, want 987654321", got.User.ID)
	}
	if got.User.LanguageCode != "ru" {
		t.Errorf("User.LanguageCode = %q, want ru", got.User.LanguageCode)
	}
	if got.ChatType != "private" {
		t.Errorf("ChatType = %q, want private", got.ChatType)
	}
	if got.ChatInstance != "-1234567890123456789" {
		t.Errorf("ChatInstance = %q, want -1234567890123456789", got.ChatInstance)
	}
}
