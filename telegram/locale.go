package telegram

// locale.go — YAML-driven per-language string registry for Telegram bots.
//
// Data-layer shape adapted from github.com/tucnak/telebot v4 (MIT):
//   layout/layout.go, layout/parser.go (selective, ≤200 LOC kept).
// Bot-specific coupling (tele.Context, markup DSL, inline results) dropped.
// See telegram/THIRD_PARTY_NOTICES.md for full attribution.

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Command represents a single Telegram bot command used in setMyCommands.
type Command struct {
	Cmd  string
	Desc string
}

// localeData holds the parsed YAML for one language.
type localeData struct {
	Strings  map[string]string `yaml:"strings"`
	Buttons  map[string]string `yaml:"buttons"`
	Commands []struct {
		Cmd  string `yaml:"cmd"`
		Desc string `yaml:"desc"`
	} `yaml:"commands"`
}

// Locale loads YAML-defined per-lang string maps, button labels, and command menus.
//
// Each locale file maps keys → strings; missing keys fall back to the default lang.
// Locale is immutable after construction; all methods are safe for concurrent use.
type Locale struct {
	defaultLang string
	langs       map[string]*localeData
}

// NewLocale loads locale YAML files from fsys (caller picks: embed.FS, os.DirFS,
// testing/fstest.MapFS, …). Files are expected at the root of fsys with names
// matching "<lang>.yaml" (e.g. "ru.yaml", "en.yaml").
//
// defaultLang must be present in fsys; it is used as the fallback for missing keys.
// Returns an error if defaultLang file is absent or any loaded file has invalid YAML.
func NewLocale(fsys fs.FS, defaultLang string) (*Locale, error) {
	langs := make(map[string]*localeData)

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("telegram/locale: read dir: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		lang := strings.TrimSuffix(name, ".yaml")

		data, err := fs.ReadFile(fsys, path.Join(".", name))
		if err != nil {
			return nil, fmt.Errorf("telegram/locale: read %s: %w", name, err)
		}

		var ld localeData
		if err := yaml.Unmarshal(data, &ld); err != nil {
			return nil, fmt.Errorf("telegram/locale: parse %s: %w", name, err)
		}

		langs[lang] = &ld
	}

	if _, ok := langs[defaultLang]; !ok {
		return nil, fmt.Errorf("telegram/locale: default lang %q not found in fs", defaultLang)
	}

	return &Locale{
		defaultLang: defaultLang,
		langs:       langs,
	}, nil
}

// Get returns the string for key in lang. If the key is missing in lang, it falls
// back to the default lang. If still missing, it returns key as-is (debug-friendly).
//
// When vars is non-empty, the string is interpreted as a text/template with the
// first element of vars as the dot value ({{.}}). Template errors return the raw
// string without substitution.
func (l *Locale) Get(lang, key string, vars ...any) string {
	raw := l.lookup(lang, key)
	if len(vars) == 0 {
		return raw
	}
	t, err := template.New("").Parse(raw)
	if err != nil {
		return raw
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars[0]); err != nil {
		return raw
	}
	return buf.String()
}

// lookup returns the raw (un-templated) string for key in lang, with default-lang
// fallback and final key-as-sentinel fallback.
func (l *Locale) lookup(lang, key string) string {
	if ld, ok := l.langs[lang]; ok {
		if v, ok := ld.Strings[key]; ok {
			return v
		}
	}
	// Fallback to default lang.
	if ld, ok := l.langs[l.defaultLang]; ok {
		if v, ok := ld.Strings[key]; ok {
			return v
		}
	}
	return key // sentinel: return key for debuggability
}

// Available returns the list of locale codes that loaded successfully.
// The order is not guaranteed.
func (l *Locale) Available() []string {
	out := make([]string, 0, len(l.langs))
	for lang := range l.langs {
		out = append(out, lang)
	}
	return out
}

// Buttons returns button labels for lang. Falls back to default lang for missing keys.
func (l *Locale) Buttons(lang string) map[string]string {
	result := make(map[string]string)

	// Start with default lang buttons (base layer).
	if ld, ok := l.langs[l.defaultLang]; ok {
		for k, v := range ld.Buttons {
			result[k] = v
		}
	}

	// Overlay with requested lang (override where available).
	if lang != l.defaultLang {
		if ld, ok := l.langs[lang]; ok {
			for k, v := range ld.Buttons {
				result[k] = v
			}
		}
	}

	return result
}

// Commands returns the bot command menu for lang (used by setMyCommands).
// Falls back to default lang if lang is not loaded.
func (l *Locale) Commands(lang string) []Command {
	ld, ok := l.langs[lang]
	if !ok {
		ld = l.langs[l.defaultLang]
	}
	if ld == nil {
		return nil
	}
	out := make([]Command, 0, len(ld.Commands))
	for _, c := range ld.Commands {
		out = append(out, Command{Cmd: c.Cmd, Desc: c.Desc})
	}
	return out
}

// Button returns the label for a single button key in lang, without allocating
// a full map. Falls back to the default lang if the key is absent in lang.
// Returns key itself if absent everywhere (same sentinel behaviour as Get).
func (l *Locale) Button(lang, key string) string {
	// Check requested lang first.
	if ld, ok := l.langs[lang]; ok {
		if v, ok := ld.Buttons[key]; ok {
			return v
		}
	}
	// Fall back to default lang.
	if ld, ok := l.langs[l.defaultLang]; ok {
		if v, ok := ld.Buttons[key]; ok {
			return v
		}
	}
	return key // sentinel: return key for debuggability
}
