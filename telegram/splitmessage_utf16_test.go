package telegram

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/anatolykoptev/go-kit/strutil"
)

// validUTF16 reports whether s encodes to a valid UTF-16 stream with no
// lone surrogates. utf8.ValidString is NOT enough: a lone high surrogate
// (the first half of a split emoji) is valid UTF-8 (U+D800-DFFF are
// assigned code points) but invalid UTF-16. Telegram rejects lone
// surrogates.
func validUTF16(s string) bool {
	runes := []rune(s)
	for _, r := range runes {
		if utf16.IsSurrogate(r) {
			// A surrogate code point in a rune slice means the original
			// string had a lone surrogate — utf16.Encode will emit it as
			// a lone surrogate, which is invalid UTF-16.
			return false
		}
	}
	return true
}

// TestSplitMessage_UTF16_UnitBudget is the RED test for the rune-vs-UTF-16
// defect. Telegram measures message length in UTF-16 code units, but
// SplitMessage measured in runes. A reply of 4000 BMP runes + 100 emoji is
// 4000 + 200 = 4200 UTF-16 units, but only 4100 runes — so the rune-based
// splitter saw 4100 > 4096 and split, but a 4000-rune + 96-emoji payload
// (4096 runes, 4192 units) was returned as ONE chunk that Telegram rejects.
//
// This test uses 4000 BMP runes + 100 emoji (4100 runes, 4200 units) to
// make the defect unambiguous: every chunk must be <= 4096 UTF-16 units.
func TestSplitMessage_UTF16_UnitBudget(t *testing.T) {
	bmp := strings.Repeat("а", 4000)  // 4000 Cyrillic 'а' = 4000 UTF-16 units
	emoji := strings.Repeat("🔥", 100) // 100 emoji = 200 UTF-16 units
	msg := bmp + emoji
	// 4100 runes, 4200 UTF-16 units.

	chunks := SplitMessage(msg, MaxMessageLen)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks for 4200 UTF-16 units at maxLen=4096, got %d: %v",
			len(chunks), chunks)
	}
	for i, ch := range chunks {
		units := strutil.UTF16Len(ch)
		if units > MaxMessageLen {
			t.Errorf("chunk %d exceeds MaxMessageLen in UTF-16 units: %d > %d, chunk=%q",
				i, units, MaxMessageLen, ch)
		}
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d is not valid UTF-8: %q", i, ch)
		}
		if !validUTF16(ch) {
			t.Errorf("chunk %d contains a lone surrogate (invalid UTF-16): %q", i, ch)
		}
	}
}

// TestSplitMessage_UTF16_SurrogatePairNeverHalved places a chunk boundary
// exactly on an emoji and asserts the pair is never split.
func TestSplitMessage_UTF16_SurrogatePairNeverHalved(t *testing.T) {
	// 4095 BMP chars + 1 emoji = 4095 runes + 1 rune = 4096 runes,
	// but 4095 + 2 = 4097 UTF-16 units. The boundary must land so the
	// emoji goes wholly into one chunk, never halved.
	emoji := "🔥"
	msg := strings.Repeat("a", 4095) + emoji
	chunks := SplitMessage(msg, MaxMessageLen)

	for i, ch := range chunks {
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8: %q", i, ch)
		}
		if !validUTF16(ch) {
			t.Errorf("chunk %d has lone surrogate: %q", i, ch)
		}
		units := strutil.UTF16Len(ch)
		if units > MaxMessageLen {
			t.Errorf("chunk %d exceeds %d UTF-16 units: %d", i, MaxMessageLen, units)
		}
	}
	// The emoji must appear intact in exactly one chunk.
	emojiChunks := 0
	for _, ch := range chunks {
		if strings.Contains(ch, emoji) {
			emojiChunks++
		}
	}
	if emojiChunks != 1 {
		t.Errorf("emoji should appear intact in exactly 1 chunk, found in %d chunks", emojiChunks)
	}
}

// TestSplitMessage_UTF16_HTMLTagsReopenedStillWithinLimit verifies that
// reopened HTML tags don't push a chunk back over the UTF-16 limit.
func TestSplitMessage_UTF16_HTMLTagsReopenedStillWithinLimit(t *testing.T) {
	// <b> tag spanning a boundary, with emoji content to stress UTF-16.
	content := strings.Repeat("🔥", 100) + "\n" + strings.Repeat("🎉", 100)
	msg := "<b>" + content + "</b>"
	chunks := SplitMessage(msg, 100)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
	for i, ch := range chunks {
		units := strutil.UTF16Len(ch)
		if units > 100 {
			t.Errorf("chunk %d exceeds maxLen 100 in UTF-16 units: %d, chunk=%q", i, units, ch)
		}
		if !validUTF16(ch) {
			t.Errorf("chunk %d has lone surrogate: %q", i, ch)
		}
	}
	// First chunk must close <b>, second must reopen.
	if !strings.Contains(chunks[0], "</b>") {
		t.Errorf("first chunk missing </b>: %q", chunks[0])
	}
	if !strings.Contains(chunks[1], "<b>") {
		t.Errorf("second chunk missing <b>: %q", chunks[1])
	}
}
