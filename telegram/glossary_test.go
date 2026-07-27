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

// A stray '<' (not followed by a letter or '/') must be treated as a literal
// character, not a tag start. Otherwise a lone '<' with no '>' drops all later
// terms (the loop breaks), and '< ютуб >' swallows ютуб as a fake tag.
func TestGlossary_StrayAngleBracket(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "YouTube", Aliases: []string{"ютуб"}},
	})
	tests := []struct{ name, in, want string }{
		{"stray lt before gt", "темп < 5 > ютуб", "темп < 5 > YouTube"},
		{"stray lt swallows term", "темп < ютуб >", "темп < YouTube >"},
		{"lone lt no gt drops later", "ютуб < ютуб", "YouTube < YouTube"},
		{"trailing lone lt", "ютуб <", "YouTube <"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := g.Apply(tc.in); got != tc.want {
				t.Errorf("Apply(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Apply is idempotent: applying twice yields the same result as applying once.
// Guards against double-<b> wrapping and re-mangling of already-normalized text.
func TestGlossary_SecondApplyIdempotent(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "HeadHunter", Aliases: []string{"хэт хантер"}, Bold: true},
		{Canonical: "YouTube", Aliases: []string{"ютуб"}, Bold: false},
	})
	in := "хэт хантер ютуб"
	once := g.Apply(in)
	twice := g.Apply(once)
	if once != twice {
		t.Errorf("not idempotent:\n once  = %q\n twice = %q", once, twice)
	}
}

// When two aliases start at the same position, the longer one is tried first.
// If it fails the trailing word-boundary, the shorter one must still match at
// that same position — it must not be skipped.
func TestGlossary_ShorterWinsAtSamePosition(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "Chat", Aliases: []string{"чат", "чат бот"}},
	})
	if got := g.Apply("чат ботинок"); got != "Chat ботинок" {
		t.Errorf("shorter wins: got %q, want %q", got, "Chat ботинок")
	}
}

// isWordRune includes digits, so a digit alias matches as a whole token but
// must NOT match inside a longer alphanumeric word (word-boundary aware).
func TestGlossary_DigitBoundary(t *testing.T) {
	g := NewGlossary([]Term{
		{Canonical: "Three", Aliases: []string{"3"}},
	})
	if got := g.Apply("3"); got != "Three" {
		t.Errorf("bare digit: got %q, want %q", got, "Three")
	}
	if got := g.Apply("в3"); got != "в3" {
		t.Errorf("digit inside word: got %q, want %q", got, "в3")
	}
	if got := g.Apply("3d"); got != "3d" {
		t.Errorf("letter after digit: got %q, want %q", got, "3d")
	}
}

// Empty canonical and blank/whitespace-only aliases are silently ignored — no
// panic, no spurious match.
func TestGlossary_EmptyAndBlankNoOp(t *testing.T) {
	// Empty canonical: the whole term (including its aliases) is skipped.
	g := NewGlossary([]Term{
		{Canonical: "", Aliases: []string{"ютуб"}},
	})
	if got := g.Apply("ютуб here"); got != "ютуб here" {
		t.Errorf("empty canonical: got %q, want no match", got)
	}
	// Whitespace-only / empty aliases are ignored; canonical still registered.
	g2 := NewGlossary([]Term{
		{Canonical: "YouTube", Aliases: []string{"  ", "\t", ""}},
	})
	if got := g2.Apply("   "); got != "   " {
		t.Errorf("blank alias on blank input: got %q", got)
	}
	if got := g2.Apply("youtube"); got != "YouTube" {
		t.Errorf("canonical still works: got %q, want %q", got, "YouTube")
	}
}
