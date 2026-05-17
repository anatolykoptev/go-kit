package callback

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Sentinel errors returned by Codec methods.
var (
	// ErrInvalidSignature is returned when the HMAC does not match.
	// Uses crypto/subtle.ConstantTimeCompare to prevent timing side-channels.
	ErrInvalidSignature = errors.New("callback: invalid signature")

	// ErrTooLarge is returned when the encoded string would exceed the
	// Telegram 64-byte CallbackData limit.
	ErrTooLarge = errors.New("callback: encoded data exceeds 64 bytes")

	// ErrMalformed is returned when the input does not match the expected
	// wire format (prefix:b64payload:b64sig).
	ErrMalformed = errors.New("callback: malformed encoding")
)

const (
	// telegramMaxCallbackBytes is the hard limit imposed by Telegram on
	// inline keyboard callback_data values.
	telegramMaxCallbackBytes = 64

	// hmacTruncateBytes is the number of HMAC-SHA256 bytes included in the
	// signature. 8 bytes = 64-bit forgery cost ≈ 2^64 under a uniform key.
	hmacTruncateBytes = 8
)

// b64 is the URL-safe, no-padding base64 encoding used throughout.
var b64 = base64.URLEncoding.WithPadding(base64.NoPadding)

// Codec signs and verifies Telegram CallbackData using HMAC-SHA256.
// The zero value is not usable; create via [New].
type Codec struct {
	secret []byte
}

// New creates a Codec using the given secret.
// The secret should be derived from the bot token + an application-specific
// salt; it must not be empty in production.
func New(secret []byte) *Codec {
	s := make([]byte, len(secret))
	copy(s, secret)
	return &Codec{secret: s}
}

// Encode produces a signed CallbackData string in the format:
//
//	<prefix>:<base64url(payload)>:<base64url(hmac[:8])>
//
// Returns ErrTooLarge if the result exceeds 64 bytes (Telegram limit).
// Returns an error if prefix contains ':', which would make decoding ambiguous.
func (c *Codec) Encode(prefix string, payload []byte) (string, error) {
	if strings.ContainsRune(prefix, ':') {
		return "", errors.New("callback: prefix must not contain ':'")
	}

	b64Payload := b64.EncodeToString(payload)

	// HMAC input binds both prefix and payload to prevent swapping attacks.
	macInput := prefix + ":" + b64Payload
	sig := c.computeMAC(macInput)
	b64Sig := b64.EncodeToString(sig)

	encoded := macInput + ":" + b64Sig
	if len(encoded) > telegramMaxCallbackBytes {
		return "", ErrTooLarge
	}
	return encoded, nil
}

// Decode verifies the signature and returns the prefix and original payload.
//
// Returns:
//   - [ErrInvalidSignature] if the HMAC does not match (uses constant-time compare).
//   - [ErrMalformed] if the input does not have exactly three colon-separated parts.
func (c *Codec) Decode(callbackData string) (prefix string, payload []byte, err error) {
	parts := strings.SplitN(callbackData, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return "", nil, ErrMalformed
	}

	prefix = parts[0]
	b64Payload := parts[1]
	b64Sig := parts[2]

	// Reconstruct the MAC input and verify before decoding payload.
	macInput := prefix + ":" + b64Payload
	expectedSig := c.computeMAC(macInput)

	gotSig, err := b64.DecodeString(b64Sig)
	if err != nil {
		return "", nil, ErrMalformed
	}
	if len(gotSig) != hmacTruncateBytes {
		return "", nil, ErrMalformed
	}

	// Constant-time comparison prevents timing side-channels on the MAC.
	if subtle.ConstantTimeCompare(expectedSig, gotSig) != 1 {
		return "", nil, ErrInvalidSignature
	}

	payload, err = b64.DecodeString(b64Payload)
	if err != nil {
		return "", nil, ErrMalformed
	}
	return prefix, payload, nil
}

// computeMAC returns the first [hmacTruncateBytes] bytes of HMAC-SHA256(secret, input).
func (c *Codec) computeMAC(input string) []byte {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(input))
	full := mac.Sum(nil)
	return full[:hmacTruncateBytes]
}

// EncodeTyped marshals v to JSON and calls [Codec.Encode].
// Useful for struct payloads; relies on json.Marshal for serialization.
func EncodeTyped[T any](c *Codec, prefix string, v T) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return c.Encode(prefix, data)
}

// DecodeTyped calls [Codec.Decode] and unmarshals the payload into T via JSON.
func DecodeTyped[T any](c *Codec, callbackData string) (prefix string, v T, err error) {
	prefix, payload, err := c.Decode(callbackData)
	if err != nil {
		return "", v, err
	}
	if err = json.Unmarshal(payload, &v); err != nil {
		return "", v, err
	}
	return prefix, v, nil
}
