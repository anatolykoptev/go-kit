# strutil: Additions Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add TruncateMiddle, configurable placeholder (*With variants), and case conversions (ToSnakeCase, ToCamelCase, ToKebabCase, ToPascalCase) — completing Q1 strutil roadmap items.

**Architecture:** All functions added to strutil.go. Existing Truncate/TruncateAtWord refactored as thin wrappers around *With variants. Case conversions use a shared splitWords helper. stdlib only (`unicode`).

**Tech Stack:** Go stdlib only (`strings`, `unicode`)

---

### Task 1: All strutil code additions

**Files:** strutil/strutil.go

**Add `"unicode"` to imports.**

#### 1a. Configurable placeholder (*With variants)

Refactor existing functions as wrappers. Add TruncateWith, TruncateAtWordWith:

```go
// Truncate caps s at maxRunes runes, appending "..." if truncated.
func Truncate(s string, maxRunes int) string {
	return TruncateWith(s, maxRunes, "...")
}

// TruncateWith is like Truncate but uses a custom placeholder.
func TruncateWith(s string, maxRunes int, placeholder string) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + placeholder
}

// TruncateAtWord truncates s to maxRunes at a word boundary, appending "...".
func TruncateAtWord(s string, maxRunes int) string {
	return TruncateAtWordWith(s, maxRunes, "...")
}

// TruncateAtWordWith is like TruncateAtWord but uses a custom placeholder.
func TruncateAtWordWith(s string, maxRunes int, placeholder string) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	truncated := string(runes[:maxRunes])
	cut := strings.LastIndex(truncated, " ")
	if cut < len(truncated)/2 {
		return truncated + placeholder
	}
	return truncated[:cut] + placeholder
}
```

#### 1b. TruncateMiddle

```go
// TruncateMiddle keeps the start and end of s, cutting the middle.
// Appends "..." as the middle placeholder.
// Example: TruncateMiddle("path/to/very/long/file.go", 15) → "path/to/...ile.go"
func TruncateMiddle(s string, maxRunes int) string {
	return TruncateMiddleWith(s, maxRunes, "...")
}

// TruncateMiddleWith is like TruncateMiddle but uses a custom placeholder.
func TruncateMiddleWith(s string, maxRunes int, placeholder string) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 0 {
		return placeholder
	}
	head := (maxRunes + 1) / 2
	tail := maxRunes - head
	if tail <= 0 {
		return string(runes[:head]) + placeholder
	}
	return string(runes[:head]) + placeholder + string(runes[len(runes)-tail:])
}
```

#### 1c. Case conversions

```go
// splitWords breaks s into words by detecting case transitions and delimiters.
// Handles camelCase, PascalCase, snake_case, kebab-case, spaces, and acronyms.
func splitWords(s string) []string {
	var words []string
	runes := []rune(s)
	start := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if r == '_' || r == '-' || r == ' ' {
			if i > start {
				words = append(words, string(runes[start:i]))
			}
			start = i + 1
			continue
		}

		if i > start && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				words = append(words, string(runes[start:i]))
				start = i
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				words = append(words, string(runes[start:i]))
				start = i
			}
		}
	}
	if start < len(runes) {
		words = append(words, string(runes[start:]))
	}
	return words
}

// ToSnakeCase converts s to snake_case.
// "myVariableName" → "my_variable_name", "HTTPServer" → "http_server"
func ToSnakeCase(s string) string {
	words := splitWords(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

// ToKebabCase converts s to kebab-case.
// "myVariableName" → "my-variable-name", "HTTPServer" → "http-server"
func ToKebabCase(s string) string {
	words := splitWords(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "-")
}

// ToCamelCase converts s to camelCase.
// "my_variable_name" → "myVariableName", "HTTPServer" → "httpServer"
func ToCamelCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}
	words[0] = strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		words[i] = titleWord(words[i])
	}
	return strings.Join(words, "")
}

// ToPascalCase converts s to PascalCase.
// "my_variable_name" → "MyVariableName", "http_server" → "HttpServer"
func ToPascalCase(s string) string {
	words := splitWords(s)
	for i := range words {
		words[i] = titleWord(words[i])
	}
	return strings.Join(words, "")
}

// titleWord returns w with the first rune upper-cased and the rest lower-cased.
func titleWord(w string) string {
	runes := []rune(w)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	for j := 1; j < len(runes); j++ {
		runes[j] = unicode.ToLower(runes[j])
	}
	return string(runes)
}
```

**Step 1:** Write the complete updated strutil.go.

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./strutil/ -v -count=1
```
Expected: All 11 existing tests PASS (refactored functions are backward-compatible).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add strutil/strutil.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "strutil: add TruncateMiddle, *With placeholders, case conversions

- TruncateMiddle/TruncateMiddleWith: keep start+end, cut middle
- TruncateWith/TruncateAtWordWith: configurable placeholder
- ToSnakeCase, ToCamelCase, ToKebabCase, ToPascalCase
- splitWords helper handles camelCase, acronyms, delimiters

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new functions

**Files:** strutil/strutil_test.go

Add tests for each new function.

**TruncateMiddle tests:**

```go
func TestTruncateMiddle_Short(t *testing.T) {
	if got := strutil.TruncateMiddle("hello", 10); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncateMiddle_Long(t *testing.T) {
	got := strutil.TruncateMiddle("path/to/very/long/file.go", 15)
	// head=8, tail=7: "path/to/...file.go"
	want := "path/to/...file.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTruncateMiddle_Unicode(t *testing.T) {
	got := strutil.TruncateMiddle("Привет прекрасный мир", 10)
	// head=5, tail=5: "Приве...ый мир"
	if len([]rune(got)) < 10 {
		t.Errorf("got %q, should preserve start and end", got)
	}
}
```

**TruncateWith tests:**

```go
func TestTruncateWith_CustomPlaceholder(t *testing.T) {
	got := strutil.TruncateWith("hello world", 5, "[...]")
	if got != "hello[...]" {
		t.Errorf("got %q, want %q", got, "hello[...]")
	}
}

func TestTruncateMiddleWith_CustomPlaceholder(t *testing.T) {
	got := strutil.TruncateMiddleWith("hello beautiful world", 10, "~")
	// head=5, tail=5: "hello~world"
	if got != "hello~world" {
		t.Errorf("got %q, want %q", got, "hello~world")
	}
}
```

**Case conversion tests:**

```go
func TestToSnakeCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"myVariableName", "my_variable_name"},
		{"HTTPServer", "http_server"},
		{"parseJSON", "parse_json"},
		{"simple", "simple"},
		{"already_snake", "already_snake"},
		{"kebab-case", "kebab_case"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := strutil.ToSnakeCase(tt.in); got != tt.want {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"my_variable_name", "myVariableName"},
		{"HTTPServer", "httpServer"},
		{"kebab-case-name", "kebabCaseName"},
		{"simple", "simple"},
		{"PascalCase", "pascalCase"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := strutil.ToCamelCase(tt.in); got != tt.want {
			t.Errorf("ToCamelCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToKebabCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"myVariableName", "my-variable-name"},
		{"HTTPServer", "http-server"},
		{"snake_case", "snake-case"},
		{"simple", "simple"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := strutil.ToKebabCase(tt.in); got != tt.want {
			t.Errorf("ToKebabCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct{ in, want string }{
		{"my_variable_name", "MyVariableName"},
		{"httpServer", "HttpServer"},
		{"kebab-case-name", "KebabCaseName"},
		{"simple", "Simple"},
		{"already_pascal", "AlreadyPascal"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := strutil.ToPascalCase(tt.in); got != tt.want {
			t.Errorf("ToPascalCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
```

**Step 1:** Add all tests to strutil_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./strutil/ -v -count=1
```
Expected: All tests PASS (11 existing + new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add strutil/strutil_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "strutil: add tests for TruncateMiddle, *With, case conversions

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update strutil section:

```go
import "github.com/anatolykoptev/go-kit/strutil"

s := strutil.Truncate("Hello, world!", 5)       // "Hello..."
s = strutil.TruncateAtWord("Hello, world!", 8)  // "Hello,..."
s = strutil.TruncateMiddle("path/to/file.go", 10) // "path/...e.go"

// Custom placeholder
s = strutil.TruncateWith("Hello, world!", 5, "[...]")  // "Hello[...]"

// Case conversions
s = strutil.ToSnakeCase("myVariableName")  // "my_variable_name"
s = strutil.ToCamelCase("my_variable")     // "myVariable"
s = strutil.ToKebabCase("myVariableName")  // "my-variable-name"
s = strutil.ToPascalCase("my_variable")    // "MyVariable"

ok := strutil.Contains([]string{"a", "b"}, "a")    // true
ok = strutil.ContainsAny("hello world", []string{"world"}) // true
```

**ROADMAP changes:**
- Mark strutil items 1-3 as DONE

**Step 1:** Update README.md strutil section.

**Step 2:** Update ROADMAP.md strutil status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update strutil section for new features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
