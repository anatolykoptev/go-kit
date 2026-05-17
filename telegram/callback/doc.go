// Package callback provides HMAC-SHA256 signed Telegram CallbackData encoding.
//
// # Security model
//
// Telegram inline keyboard callbacks carry a free-form string up to 64 bytes.
// Without signing, an attacker can enumerate valid payload values (e.g. iterate
// partner-edge IDs) by replaying or forging callback_data. HMAC signing makes
// forgery computationally infeasible.
//
// # Wire format
//
//	<prefix>:<base64url-no-pad(payload)>:<base64url-no-pad(hmac[:8])>
//
// The HMAC-SHA256 input is the pre-signature portion of the wire string:
//
//	mac_input = prefix + ":" + base64url-no-pad(payload)
//
// Binding the prefix into the MAC prevents prefix-swapping attacks (e.g.
// turning a "confirm" callback into a "delete" callback with the same payload).
//
// # HMAC truncation
//
// Only the first 8 bytes (64 bits) of HMAC-SHA256 are included. This fits the
// 64-byte Telegram limit while providing forgery cost ≈ 2^64 operations under
// a uniform key. Per NIST SP 800-107 §5.2, truncation to ≥ 32 bits is
// acceptable for MAC use; 64 bits is well above that threshold.
//
// 8 bytes of HMAC encodes to exactly 11 base64url-no-pad characters.
// Fixed overhead per encoded string: len(prefix) + 1 + 11 + 1 = len(prefix) + 13.
// With an empty prefix the usable payload budget is ≈ 38 raw bytes (51 after b64).
//
// # Signature verification
//
// [Codec.Decode] uses [crypto/subtle.ConstantTimeCompare] to prevent timing
// side-channels on the 8-byte MAC comparison.
//
// # Usage
//
//	// Derive the secret from the bot token + env salt — never use the raw token.
//	secret := deriveSecret(botToken, os.Getenv("CALLBACK_SALT"))
//	codec := callback.New(secret)
//
//	// Encoding a struct payload:
//	data, err := callback.EncodeTyped(codec, "partner", PartnerAction{ID: 42, Op: "approve"})
//
//	// Decoding in the update handler:
//	prefix, action, err := callback.DecodeTyped[PartnerAction](codec, update.CallbackQuery.Data)
//	if err != nil {
//	    // ErrInvalidSignature → reject silently (bot-enumeration attempt)
//	    // ErrMalformed       → reject (garbage input)
//	    // ErrTooLarge        → shouldn't occur at decode; your Encode is misconfigured
//	    return
//	}
package callback
