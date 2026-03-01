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

// --- TruncateMiddle ---

func TestTruncateMiddle_Short(t *testing.T) {
	if got := strutil.TruncateMiddle("hello", 10); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTruncateMiddle_Long(t *testing.T) {
	got := strutil.TruncateMiddle("path/to/very/long/file.go", 15)
	// head=8, tail=7: "path/to/" + "..." + "file.go"
	want := "path/to/...file.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTruncateMiddle_Unicode(t *testing.T) {
	got := strutil.TruncateMiddle("Привет прекрасный мир", 10)
	// head=5, tail=5: "Приве" + "..." + "й мир"
	want := "Приве...й мир"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- TruncateWith / TruncateMiddleWith ---

func TestTruncateWith_CustomPlaceholder(t *testing.T) {
	got := strutil.TruncateWith("hello world", 5, "[...]")
	if got != "hello[...]" {
		t.Errorf("got %q, want %q", got, "hello[...]")
	}
}

func TestTruncateMiddleWith_CustomPlaceholder(t *testing.T) {
	got := strutil.TruncateMiddleWith("hello beautiful world", 10, "~")
	// head=5, tail=5: "hello" + "~" + "world"
	if got != "hello~world" {
		t.Errorf("got %q, want %q", got, "hello~world")
	}
}

// --- Case conversions ---

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
