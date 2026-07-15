package telegram

import (
	"testing"
	"testing/fstest"
)

// localeFixture returns a *Locale loaded with two languages, strings (plain + templated),
// and buttons — representative of a real keyboard-heavy bot.
func localeFixture(b *testing.B) *Locale {
	b.Helper()
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  welcome: "Добро пожаловать!"
  greeting: "Привет, {{.Name}}!"
buttons:
  confirm: "Подтвердить"
  cancel: "Отмена"
  back: "Назад"
  menu: "Меню"
`)},
		"en.yaml": {Data: []byte(`
strings:
  welcome: "Welcome!"
  greeting: "Hello, {{.Name}}!"
buttons:
  confirm: "Confirm"
  cancel: "Cancel"
  back: "Back"
  menu: "Menu"
`)},
	}
	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		b.Fatalf("NewLocale: %v", err)
	}
	return loc
}

// BenchmarkLocale_GetPlain measures Get on a key with no template vars.
// Target: zero allocations per operation.
func BenchmarkLocale_GetPlain(b *testing.B) {
	loc := localeFixture(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = loc.Get("ru", "welcome")
	}
}

// BenchmarkLocale_GetTemplated measures Get on a key with template substitution.
// The template must be pre-compiled; only Execute + buffer allocs remain.
func BenchmarkLocale_GetTemplated(b *testing.B) {
	loc := localeFixture(b)
	type vars struct{ Name string }
	data := vars{Name: "Alice"}
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = loc.Get("ru", "greeting", data)
	}
}

// BenchmarkLocale_Button measures Button(lang, key) — single-key lookup without
// building a full map. Target: zero allocations.
func BenchmarkLocale_Button(b *testing.B) {
	loc := localeFixture(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = loc.Button("ru", "confirm")
	}
}

// BenchmarkLocale_Buttons measures Buttons(lang) — the full button map used by
// keyboard renderers. With memoization the map must be shared (zero allocs).
func BenchmarkLocale_Buttons(b *testing.B) {
	loc := localeFixture(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = loc.Buttons("ru")
	}
}
