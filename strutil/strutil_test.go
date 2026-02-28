package strutil_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/strutil"
)

func TestTruncate_Short(t *testing.T) {
	if got := strutil.Truncate("hello", 10); got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
}

func TestTruncate_Exact(t *testing.T) {
	if got := strutil.Truncate("hello", 5); got != "hello" {
		t.Errorf("Truncate = %q, want %q", got, "hello")
	}
}

func TestTruncate_Long(t *testing.T) {
	got := strutil.Truncate("hello world", 5)
	if got != "hello..." {
		t.Errorf("Truncate = %q, want %q", got, "hello...")
	}
}

func TestTruncate_Unicode(t *testing.T) {
	got := strutil.Truncate("Привет мир", 6)
	if got != "Привет..." {
		t.Errorf("Truncate = %q, want %q", got, "Привет...")
	}
}

func TestTruncate_Emoji(t *testing.T) {
	got := strutil.Truncate("Hi 👋🌍!", 4)
	if got != "Hi 👋..." {
		t.Errorf("Truncate = %q, want %q", got, "Hi 👋...")
	}
}

func TestTruncate_Empty(t *testing.T) {
	if got := strutil.Truncate("", 10); got != "" {
		t.Errorf("Truncate = %q, want empty", got)
	}
}

func TestTruncateAtWord(t *testing.T) {
	got := strutil.TruncateAtWord("hello beautiful world", 15)
	if got != "hello beautiful..." {
		t.Errorf("TruncateAtWord = %q, want %q", got, "hello beautiful...")
	}
}

func TestTruncateAtWord_Short(t *testing.T) {
	if got := strutil.TruncateAtWord("short", 10); got != "short" {
		t.Errorf("TruncateAtWord = %q, want %q", got, "short")
	}
}

func TestTruncateAtWord_NoSpaceNearCut(t *testing.T) {
	got := strutil.TruncateAtWord("abcdefghijklmnop", 10)
	if got != "abcdefghij..." {
		t.Errorf("TruncateAtWord = %q, want %q", got, "abcdefghij...")
	}
}

func TestContains(t *testing.T) {
	items := []string{"go", "python", "rust"}
	if !strutil.Contains(items, "python") {
		t.Error("Contains(python) = false, want true")
	}
	if strutil.Contains(items, "java") {
		t.Error("Contains(java) = true, want false")
	}
	if strutil.Contains(nil, "go") {
		t.Error("Contains(nil, go) = true, want false")
	}
}

func TestContainsAny(t *testing.T) {
	if !strutil.ContainsAny("hello world", []string{"xyz", "world"}) {
		t.Error("ContainsAny = false, want true")
	}
	if strutil.ContainsAny("hello", []string{"xyz", "abc"}) {
		t.Error("ContainsAny = true, want false")
	}
}
