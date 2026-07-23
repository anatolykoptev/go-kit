package telegram

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Glossary
// ---------------------------------------------------------------------------

func TestGlossary_AliasToCanonical(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер", "хед хантер"}},
	})
	tests := []struct {
		name, in, want string
	}{
		{"simple alias", "я работаю в хэт хантер сейчас", "я работаю в HeadHunter сейчас"},
		{"second alias", "хед хантер платит", "HeadHunter платит"},
		{"case-insensitive upper", "ХЭТ ХАНТЕР — сайт", "HeadHunter — сайт"},
		{"case-insensitive mixed", "Хэт Хантер сайт", "HeadHunter сайт"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Apply(tc.in); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestGlossary_WrongCaseCanonical(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}},
	})
	// "headhunter" is the canonical with wrong case -> normalized to "HeadHunter".
	if got := g.Apply("headhunter is hiring"); got != "HeadHunter is hiring" {
		t.Errorf("wrong-case canonical: got %q, want %q", got, "HeadHunter is hiring")
	}
	// Already-correct canonical passes through unchanged.
	if got := g.Apply("HeadHunter is hiring"); got != "HeadHunter is hiring" {
		t.Errorf("correct canonical: got %q, want %q", got, "HeadHunter is hiring")
	}
}

func TestGlossary_CyrillicWordBoundaries(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "YouTube", Aliases: []string{"ютуб"}},
	})
	// Whole word matches.
	if got := g.Apply("смотри на ютуб видео"); got != "смотри на YouTube видео" {
		t.Errorf("whole word: got %q", got)
	}
	// Substring inside a longer word must NOT match.
	if got := g.Apply("ютубер делает видео"); got != "ютубер делает видео" {
		t.Errorf("substring should not match: got %q", got)
	}
}

func TestGlossary_MultiWordWhitespaceTolerance(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}},
	})
	// Run of whitespace between words is tolerated and normalized away.
	if got := g.Apply("хэт  хантер сайт"); got != "HeadHunter сайт" {
		t.Errorf("double space: got %q, want %q", got, "HeadHunter сайт")
	}
	if got := g.Apply("хэт\tхантер сайт"); got != "HeadHunter сайт" {
		t.Errorf("tab between words: got %q, want %q", got, "HeadHunter сайт")
	}
}

func TestGlossary_LongestAliasFirst(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "Chat", Aliases: []string{"чат"}},
		{Canonical: "ChatGPT", Aliases: []string{"чат джи пи ти"}},
	})
	// The longer multi-word alias must win over the shorter prefix alias.
	if got := g.Apply("я спросил чат джи пи ти"); got != "я спросил ChatGPT" {
		t.Errorf("longest-first: got %q, want %q", got, "я спросил ChatGPT")
	}
	// Short alias still matches when not followed by the longer one.
	if got := g.Apply("открой чат сейчас"); got != "открой Chat сейчас" {
		t.Errorf("short alias standalone: got %q, want %q", got, "открой Chat сейчас")
	}
}

func TestGlossary_BoldFlag(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
		{Canonical: "Chat", Aliases: []string{"чат"}, Bold: false},
	})
	if got := g.Apply("хэт хантер сайт"); got != "<b>HeadHunter</b> сайт" {
		t.Errorf("bold: got %q, want %q", got, "<b>HeadHunter</b> сайт")
	}
	if got := g.Apply("чат сейчас"); got != "Chat сейчас" {
		t.Errorf("non-bold: got %q, want %q", got, "Chat сейчас")
	}
}

func TestGlossary_NoDoubleWrapInsideB(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
	})
	// Already inside <b> -> must not double-wrap.
	in := "<b>хэт хантер</b> сайт"
	want := "<b>HeadHunter</b> сайт"
	if got := g.Apply(in); got != want {
		t.Errorf("double-wrap: got %q, want %q", got, want)
	}
}

func TestGlossary_NoOpSafety(t *testing.T) {
	// nil glossary.
	if got := (*Glossary)(nil).Apply("anything"); got != "anything" {
		t.Errorf("nil glossary: got %q", got)
	}
	// empty glossary.
	g := NewGlossary(nil)
	if got := g.Apply("anything"); got != "anything" {
		t.Errorf("empty glossary: got %q", got)
	}
	// empty text.
	g2 := NewGlossary([]Term{{Canonical: "X", Aliases: []string{"y"}}})
	if got := g2.Apply(""); got != "" {
		t.Errorf("empty text: got %q", got)
	}
	// no matches.
	if got := g2.Apply("nothing here"); got != "nothing here" {
		t.Errorf("no match: got %q", got)
	}
}

func TestGlossary_DoesNotTouchHTMLTags(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
	})
	// URL inside href must be untouched; text content between tags is normalized.
	// Bold inside <a> is valid Telegram HTML (only <b> suppresses double-wrap).
	in := `<a href="http://хэт-хантер.рт">хэт хантер</a> сайт`
	want := `<a href="http://хэт-хантер.рт"><b>HeadHunter</b></a> сайт`
	if got := g.Apply(in); got != want {
		t.Errorf("html tags: got %q, want %q", got, want)
	}
}

func TestGlossary_RoundTripPrepareForTelegram(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
	})
	plain := "я работаю в хэт хантер сейчас"
	applied := g.Apply(plain)
	if !strings.Contains(applied, "<b>HeadHunter</b>") {
		t.Fatalf("Apply did not bold: %q", applied)
	}
	out, mode := PrepareForTelegram(applied)
	if mode != "HTML" {
		t.Fatalf("mode = %q, want HTML", mode)
	}
	if !strings.Contains(out, "<b>HeadHunter</b>") {
		t.Errorf("<b> did not survive PrepareForTelegram: %q", out)
	}
	if strings.Contains(out, "хэт хантер") {
		t.Errorf("alias leaked through: %q", out)
	}
	// Must not be double-wrapped or corrupted.
	if strings.Contains(out, "<b><b>") {
		t.Errorf("double <b> wrapping: %q", out)
	}
}

func TestGlossary_MultipleTermsInOnePass(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
		{Canonical: "YouTube", Aliases: []string{"ютуб"}, Bold: true},
	})
	in := "хэт хантер и ютуб"
	want := "<b>HeadHunter</b> и <b>YouTube</b>"
	if got := g.Apply(in); got != want {
		t.Errorf("multi-term: got %q, want %q", got, want)
	}
}
