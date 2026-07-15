package callback_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/telegram/callback"
)

var testSecret = []byte("super-secret-key-for-testing")

// TestCodec_RoundTrip: encode then decode returns original prefix + payload.
func TestCodec_RoundTrip(t *testing.T) {
	c := callback.New(testSecret)
	payload := []byte("partner-edge-id:42")
	encoded, err := c.Encode("nav", payload)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	prefix, got, err := c.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if prefix != "nav" {
		t.Errorf("prefix: want nav, got %q", prefix)
	}
	if string(got) != string(payload) {
		t.Errorf("payload: want %q, got %q", payload, got)
	}
}

// TestCodec_InvalidSignature_Rejected: flip a bit in the sig → ErrInvalidSignature.
func TestCodec_InvalidSignature_Rejected(t *testing.T) {
	c := callback.New(testSecret)
	encoded, err := c.Encode("nav", []byte("data"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Corrupt last character of encoded string.
	b := []byte(encoded)
	last := len(b) - 1
	if b[last] == 'A' {
		b[last] = 'B'
	} else {
		b[last] = 'A'
	}
	_, _, err = c.Decode(string(b))
	if err != callback.ErrInvalidSignature {
		t.Errorf("want ErrInvalidSignature, got %v", err)
	}
}

// TestCodec_TamperedPrefix_Rejected: change prefix → ErrInvalidSignature.
func TestCodec_TamperedPrefix_Rejected(t *testing.T) {
	c := callback.New(testSecret)
	encoded, err := c.Encode("nav", []byte("data"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Replace prefix "nav" with "bad".
	tampered := strings.Replace(encoded, "nav:", "bad:", 1)
	_, _, err = c.Decode(tampered)
	if err != callback.ErrInvalidSignature {
		t.Errorf("want ErrInvalidSignature, got %v", err)
	}
}

// TestCodec_TamperedPayload_Rejected: modify b64 payload bytes → ErrInvalidSignature.
func TestCodec_TamperedPayload_Rejected(t *testing.T) {
	c := callback.New(testSecret)
	encoded, err := c.Encode("nav", []byte("original-data"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Split into parts and modify the payload segment.
	parts := strings.SplitN(encoded, ":", 3)
	if len(parts) != 3 {
		t.Fatalf("unexpected format: %q", encoded)
	}
	pb := []byte(parts[1])
	if pb[0] == 'A' {
		pb[0] = 'B'
	} else {
		pb[0] = 'A'
	}
	tampered := parts[0] + ":" + string(pb) + ":" + parts[2]
	_, _, err = c.Decode(tampered)
	if err != callback.ErrInvalidSignature {
		t.Errorf("want ErrInvalidSignature, got %v", err)
	}
}

// TestCodec_OversizedPayload_Errors: payload that makes total > 64 bytes → ErrTooLarge.
func TestCodec_OversizedPayload_Errors(t *testing.T) {
	c := callback.New(testSecret)
	// 50 bytes of payload will push well past 64 total.
	big := make([]byte, 50)
	for i := range big {
		big[i] = 'x'
	}
	_, err := c.Encode("nav", big)
	if err != callback.ErrTooLarge {
		t.Errorf("want ErrTooLarge, got %v", err)
	}
}

// TestCodec_DifferentSecrets_Rejected: encode with secret A, decode with secret B → ErrInvalidSignature.
func TestCodec_DifferentSecrets_Rejected(t *testing.T) {
	a := callback.New([]byte("secret-a"))
	b := callback.New([]byte("secret-b"))
	encoded, err := a.Encode("nav", []byte("data"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, _, err = b.Decode(encoded)
	if err != callback.ErrInvalidSignature {
		t.Errorf("want ErrInvalidSignature, got %v", err)
	}
}

// TestCodec_ConstantTimeCompare: source-grep test — verifies subtle.ConstantTimeCompare
// is used in the implementation (not bytes.Equal or ==). This is a security invariant:
// timing-safe comparison prevents oracle attacks on the 8-byte truncated HMAC.
func TestCodec_ConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("codec.go")
	if err != nil {
		t.Fatalf("cannot read codec.go: %v", err)
	}
	if !strings.Contains(string(src), "subtle.ConstantTimeCompare") {
		t.Error("codec.go must use crypto/subtle.ConstantTimeCompare for signature verification")
	}
}

// TestEncodeTyped_DecodeTyped_RoundTrip: struct payload via JSON.
// Field names and values are kept short to fit within the 64-byte TG limit.
// Example: {"id":42,"op":"ok"} = 16 bytes raw → 22 b64 chars → "p:" + 22 + ":" + 11 = 36 total.
type testPayload struct {
	ID int64  `json:"id"`
	Op string `json:"op"`
}

func TestEncodeTyped_DecodeTyped_RoundTrip(t *testing.T) {
	c := callback.New(testSecret)
	orig := testPayload{ID: 42, Op: "ok"}
	encoded, err := callback.EncodeTyped(c, "p", orig)
	if err != nil {
		t.Fatalf("EncodeTyped: %v", err)
	}
	if len(encoded) > 64 {
		t.Errorf("encoded length %d exceeds TG 64-byte limit", len(encoded))
	}
	prefix, got, err := callback.DecodeTyped[testPayload](c, encoded)
	if err != nil {
		t.Fatalf("DecodeTyped: %v", err)
	}
	if prefix != "p" {
		t.Errorf("prefix: want p, got %q", prefix)
	}
	if got.ID != orig.ID || got.Op != orig.Op {
		t.Errorf("payload mismatch: want %+v, got %+v", orig, got)
	}

	// Also verify via raw JSON equivalence.
	wantJSON, _ := json.Marshal(orig)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("json mismatch: want %s, got %s", wantJSON, gotJSON)
	}
}

// TestCodec_MalformedInput_Returns_ErrMalformed: random garbage → ErrMalformed.
func TestCodec_MalformedInput_Returns_ErrMalformed(t *testing.T) {
	c := callback.New(testSecret)
	cases := []string{
		"",
		"noColons",
		"one:colon",
		":::extra",
	}
	for _, tc := range cases {
		_, _, err := c.Decode(tc)
		if err != callback.ErrMalformed {
			t.Errorf("Decode(%q): want ErrMalformed, got %v", tc, err)
		}
	}
}

// TestCodec_PrefixWithColon_RejectedAtEncode: `:` in prefix is ambiguous → error at Encode.
func TestCodec_PrefixWithColon_RejectedAtEncode(t *testing.T) {
	c := callback.New(testSecret)
	_, err := c.Encode("bad:prefix", []byte("data"))
	if err == nil {
		t.Error("expected error for prefix containing ':'")
	}
}

// TestCodec_EmptyPayload: empty payload is valid (prefix-only signals).
func TestCodec_EmptyPayload(t *testing.T) {
	c := callback.New(testSecret)
	encoded, err := c.Encode("cmd", []byte{})
	if err != nil {
		t.Fatalf("Encode empty payload: %v", err)
	}
	prefix, payload, err := c.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if prefix != "cmd" {
		t.Errorf("prefix: want cmd, got %q", prefix)
	}
	if len(payload) != 0 {
		t.Errorf("payload: want empty, got %q", payload)
	}
}
