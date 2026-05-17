package telegram

import (
	"testing"
	"testing/fstest"
)

// TestLocale_LoadsTwoLangs verifies that NewLocale loads RU + EN locales from an fs.FS
// and Get returns the correct string for each language.
func TestLocale_LoadsTwoLangs(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  welcome: "Добро пожаловать!"
buttons:
  domains: "🔗 Получить адрес"
commands:
  - cmd: start
    desc: "Запустить бота"
`)},
		"en.yaml": {Data: []byte(`
strings:
  welcome: "Welcome!"
buttons:
  domains: "🔗 Get address"
commands:
  - cmd: start
    desc: "Start the bot"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	if got := loc.Get("ru", "welcome"); got != "Добро пожаловать!" {
		t.Errorf("RU welcome = %q, want %q", got, "Добро пожаловать!")
	}
	if got := loc.Get("en", "welcome"); got != "Welcome!" {
		t.Errorf("EN welcome = %q, want %q", got, "Welcome!")
	}

	avail := loc.Available()
	if len(avail) != 2 {
		t.Errorf("Available() len = %d, want 2", len(avail))
	}
}

// TestLocale_FallsBackToDefault verifies that Get falls back to the default lang
// when a key is missing in the requested lang.
func TestLocale_FallsBackToDefault(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  welcome: "Добро пожаловать!"
  only_in_ru: "только по-русски"
`)},
		"en.yaml": {Data: []byte(`
strings:
  welcome: "Welcome!"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	// "only_in_ru" is missing in EN — must fall back to RU value.
	if got := loc.Get("en", "only_in_ru"); got != "только по-русски" {
		t.Errorf("fallback = %q, want %q", got, "только по-русски")
	}
}

// TestLocale_TemplatesVars verifies that Get substitutes {{.}} with the provided var.
func TestLocale_TemplatesVars(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  greeting: "Привет, {{.}}!"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	if got := loc.Get("ru", "greeting", "Alice"); got != "Привет, Alice!" {
		t.Errorf("template = %q, want %q", got, "Привет, Alice!")
	}
}

// TestLocale_MissingDefaultErrors verifies that NewLocale returns an error when
// the default lang YAML file is absent.
func TestLocale_MissingDefaultErrors(t *testing.T) {
	fsys := fstest.MapFS{
		"en.yaml": {Data: []byte(`strings:
  welcome: "Welcome!"
`)},
	}

	_, err := NewLocale(fsys, "ru")
	if err == nil {
		t.Error("expected error when default lang file missing, got nil")
	}
}

// TestLocale_MalformedYAML verifies that NewLocale returns a wrapped error for broken YAML.
func TestLocale_MalformedYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`strings: [broken: yaml: {`)},
	}

	_, err := NewLocale(fsys, "ru")
	if err == nil {
		t.Error("expected error for malformed YAML, got nil")
	}
}

// TestLocale_CommandsParsed verifies that Commands returns the correct Cmd + Desc pairs.
func TestLocale_CommandsParsed(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
commands:
  - cmd: start
    desc: "Запустить бота"
  - cmd: domains
    desc: "Получить адрес"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	cmds := loc.Commands("ru")
	if len(cmds) != 2 {
		t.Fatalf("Commands len = %d, want 2", len(cmds))
	}
	if cmds[0].Cmd != "start" || cmds[0].Desc != "Запустить бота" {
		t.Errorf("cmd[0] = %+v, want {start, Запустить бота}", cmds[0])
	}
	if cmds[1].Cmd != "domains" || cmds[1].Desc != "Получить адрес" {
		t.Errorf("cmd[1] = %+v, want {domains, Получить адрес}", cmds[1])
	}
}

// TestLocale_CachedTemplate_NoReparse verifies that the compiled *template.Template
// for a given (lang, key) is the same pointer across multiple Get calls — i.e. no
// re-parse after the first call.
func TestLocale_CachedTemplate_NoReparse(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  greeting: "Привет, {{.}}!"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	// Retrieve the compiled template twice and assert pointer identity.
	t1 := loc.compiledTemplate("ru", "greeting")
	t2 := loc.compiledTemplate("ru", "greeting")
	if t1 == nil {
		t.Fatal("compiledTemplate returned nil for a templated key")
	}
	if t1 != t2 {
		t.Errorf("compiledTemplate returned different *template.Template pointers: %p vs %p; template was re-parsed", t1, t2)
	}
}

// TestLocale_ConcurrentReads_RaceClean verifies that concurrent calls to Get and
// Buttons from multiple goroutines produce no data races (run with -race).
func TestLocale_ConcurrentReads_RaceClean(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  greeting: "Привет, {{.}}!"
  plain: "Просто строка"
buttons:
  confirm: "Подтвердить"
`)},
		"en.yaml": {Data: []byte(`
strings:
  greeting: "Hello, {{.}}!"
  plain: "Plain string"
buttons:
  confirm: "Confirm"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	const goroutines = 20
	done := make(chan struct{}, goroutines)
	for i := range goroutines {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			lang := "ru"
			if i%2 == 0 {
				lang = "en"
			}
			_ = loc.Get(lang, "greeting", "World")
			_ = loc.Get(lang, "plain")
			_ = loc.Buttons(lang)
			_ = loc.Button(lang, "confirm")
		}(i)
	}
	for range goroutines {
		<-done
	}
}

// TestLocale_NonTemplatedKey_NoTemplateOverhead verifies that plain strings (no
// template syntax) are returned directly without being stored as compiled templates
// — they must not appear in compiledTemplate.
func TestLocale_NonTemplatedKey_NoTemplateOverhead(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
strings:
  plain: "Просто строка"
  templated: "Привет, {{.}}!"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	// Plain key must not return a compiled template.
	if tmpl := loc.compiledTemplate("ru", "plain"); tmpl != nil {
		t.Errorf("compiledTemplate for plain key returned non-nil %p, want nil", tmpl)
	}

	// Templated key must return a compiled template.
	if tmpl := loc.compiledTemplate("ru", "templated"); tmpl == nil {
		t.Error("compiledTemplate for templated key returned nil, want non-nil")
	}

	// Get on the plain key must still return the raw string correctly.
	if got := loc.Get("ru", "plain"); got != "Просто строка" {
		t.Errorf("Get(plain) = %q, want %q", got, "Просто строка")
	}
}

// TestLocale_Button_SingleKey verifies the new Button(lang, key) accessor
// returns the correct label without allocating a full map (item 1.3 — v0.57 polish).
func TestLocale_Button_SingleKey(t *testing.T) {
	fsys := fstest.MapFS{
		"ru.yaml": {Data: []byte(`
buttons:
  confirm: "Подтвердить"
  cancel: "Отмена"
`)},
		"en.yaml": {Data: []byte(`
buttons:
  confirm: "Confirm"
`)},
	}

	loc, err := NewLocale(fsys, "ru")
	if err != nil {
		t.Fatalf("NewLocale: %v", err)
	}

	// Known key in the requested lang.
	if got := loc.Button("en", "confirm"); got != "Confirm" {
		t.Errorf("Button(en, confirm) = %q, want %q", got, "Confirm")
	}

	// Key missing in EN — must fall back to RU.
	if got := loc.Button("en", "cancel"); got != "Отмена" {
		t.Errorf("Button(en, cancel) = %q, want %q (fallback)", got, "Отмена")
	}

	// Key present in default lang.
	if got := loc.Button("ru", "confirm"); got != "Подтвердить" {
		t.Errorf("Button(ru, confirm) = %q, want %q", got, "Подтвердить")
	}

	// Totally unknown key — must return key as sentinel.
	if got := loc.Button("ru", "no_such_key"); got != "no_such_key" {
		t.Errorf("Button(ru, no_such_key) = %q, want sentinel %q", got, "no_such_key")
	}
}
