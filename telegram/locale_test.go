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
