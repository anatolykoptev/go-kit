package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSplitMessage_MaxLenOne_LeadingEmoji_Terminates proves the hang fixed
// by this commit. Before the fix, splitRawChunks made no forward progress
// when maxLen==1 and the text began with a non-BMP rune: UTF16ByteCut(s, 1)
// returned 0 (a 2-unit emoji doesn't fit in 1 unit), the loop appended an
// empty chunk and never advanced. This test runs under a -timeout and
// asserts (a) the call returns, and (b) every input rune appears in the
// output — nothing is silently dropped.
func TestSplitMessage_MaxLenOne_LeadingEmoji_Terminates(t *testing.T) {
	msg := "🔥hello🔥" // leading non-BMP rune, BMP middle, trailing non-BMP
	chunks := SplitMessage(msg, 1)

	// Every rune of the input must appear in the concatenation of chunks.
	got := strings.Join(chunks, "")
	wantRunes := []rune(msg)
	gotRunes := []rune(got)
	if len(gotRunes) != len(wantRunes) {
		t.Fatalf("rune loss: input has %d runes, output has %d; in=%q out=%q chunks=%v",
			len(wantRunes), len(gotRunes), msg, got, chunks)
	}
	for i, r := range wantRunes {
		if gotRunes[i] != r {
			t.Fatalf("rune mismatch at %d: want %U, got %U; out=%q", i, r, gotRunes[i], got)
		}
	}
	for i, ch := range chunks {
		if ch == "" {
			t.Errorf("chunk %d is empty", i)
		}
		if !utf8.ValidString(ch) {
			t.Errorf("chunk %d invalid UTF-8: %q", i, ch)
		}
		if !validUTF16(ch) {
			t.Errorf("chunk %d has lone surrogate: %q", i, ch)
		}
	}
}

// TestSplitMessage_SmallBudget_Invariants exhausts small maxLen values
// (1..8) against a corpus mixing BMP, non-BMP, newlines, HTML tags, and a
// leading emoji. For every (maxLen, input) pair: SplitMessage returns, no
// chunk is empty, no chunk contains a lone surrogate, and (for inputs
// without HTML tags) concatenation keeps every text rune. A single
// hand-picked case would miss the off-by-one that this class of bug hides
// behind.
//
// Content preservation is checked only for tag-free inputs: when a chunk's
// closing-tag overhead pushes it over a tiny maxLen, trimChunkToLimit must
// drop some content to fit (the alternative — emitting over-limit chunks
// — would break the UTF-16 unit limit the 4096 production budget depends
// on). That trade-off is a pre-existing design limitation of the two-pass
// split/repair pipeline, not the hang this commit fixes; it is documented
// in trimChunkToLimit's doc comment and excluded from the invariant here.
func TestSplitMessage_SmallBudget_Invariants(t *testing.T) {
	// tagFree inputs: content preservation is asserted (every rune
	// round-trips). htmlInputs: only returns/no-empty/no-surrogate are
	// asserted (tag overhead can force content loss at tiny maxLen).
	tagFree := []string{
		"🔥hello🔥",
		"🔥",
		strings.Repeat("🔥", 5),
		"🔥\n🔥\n🔥",
		"\n🔥\n🎉\n",
		"a🔥b🎉c",
		"🔥🔥\n\n🎉🎉",
	}
	htmlInputs := []string{
		"<b>🔥bold🔥</b>",
		"🔥<b>tag</b>🎉",
		"<b>" + strings.Repeat("🔥", 4) + "</b>",
	}

	// stripNL strips newlines (splitRawChunks trims boundary newlines by
	// design); tags are absent from tagFree inputs so no tag stripping is
	// needed for the content-preservation check.
	stripNL := func(s string) string { return strings.ReplaceAll(s, "\n", "") }

	for _, in := range tagFree {
		wantPlain := stripNL(in)
		for maxLen := 1; maxLen <= 8; maxLen++ {
			chunks := SplitMessage(in, maxLen)
			for i, ch := range chunks {
				if ch == "" {
					t.Errorf("maxLen=%d in=%q: chunk %d is empty", maxLen, in, i)
				}
				if !validUTF16(ch) {
					t.Errorf("maxLen=%d in=%q: chunk %d has lone surrogate: %q", maxLen, in, i, ch)
				}
				if !utf8.ValidString(ch) {
					t.Errorf("maxLen=%d in=%q: chunk %d invalid UTF-8: %q", maxLen, in, i, ch)
				}
			}
			gotPlain := stripNL(strings.Join(chunks, ""))
			if gotPlain != wantPlain {
				t.Errorf("maxLen=%d in=%q: content loss — want %q, got %q",
					maxLen, in, wantPlain, gotPlain)
			}
		}
	}

	for _, in := range htmlInputs {
		for maxLen := 1; maxLen <= 8; maxLen++ {
			chunks := SplitMessage(in, maxLen)
			for i, ch := range chunks {
				if ch == "" {
					t.Errorf("maxLen=%d in=%q: chunk %d is empty", maxLen, in, i)
				}
				if !validUTF16(ch) {
					t.Errorf("maxLen=%d in=%q: chunk %d has lone surrogate: %q", maxLen, in, i, ch)
				}
				if !utf8.ValidString(ch) {
					t.Errorf("maxLen=%d in=%q: chunk %d invalid UTF-8: %q", maxLen, in, i, ch)
				}
			}
		}
	}
}
