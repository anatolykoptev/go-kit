package langdetect

import (
	"testing"

	"github.com/RadhiFadlillah/whatlanggo"
)

func TestDetectEnglish(t *testing.T) {
	lang, conf := Detect("The quick brown fox jumps over the lazy dog. This is a well-known English pangram that contains every letter of the alphabet.")
	if lang != LangEN {
		t.Errorf("expected en, got %s (conf %.2f)", lang, conf)
	}
	if conf < 0.9 {
		t.Errorf("expected high confidence, got %.2f", conf)
	}
}

func TestDetectRussian(t *testing.T) {
	lang, conf := Detect("Привет, мир! Как дела сегодня? Я хочу рассказать тебе интересную историю о том, как я провёл выходные в Санкт-Петербурге.")
	if lang != LangRU {
		t.Errorf("expected ru, got %s (conf %.2f)", lang, conf)
	}
	if conf < 0.9 {
		t.Errorf("expected high confidence, got %.2f", conf)
	}
}

func TestDetectChinese(t *testing.T) {
	lang, conf := Detect("你好世界，今天天气很好。我想去公园散步，然后去图书馆看书。")
	if lang != LangZH {
		t.Errorf("expected zh, got %s (conf %.2f)", lang, conf)
	}
	if conf < 0.9 {
		t.Errorf("expected high confidence, got %.2f", conf)
	}
}

func TestDetectSpanish(t *testing.T) {
	lang, conf := Detect("Hola mundo, ¿cómo estás hoy? Espero que tengas un buen día. Quiero ir al parque este fin de semana para caminar.")
	if lang != LangES {
		t.Errorf("expected es, got %s (conf %.2f)", lang, conf)
	}
}

func TestDetectJapanese(t *testing.T) {
	lang, _ := Detect("こんにちは世界")
	if lang != LangJA {
		t.Errorf("expected ja, got %s", lang)
	}
}

func TestDetectEmpty(t *testing.T) {
	lang, conf := Detect("")
	if lang != LangUnknown {
		t.Errorf("expected empty for empty string, got %s", lang)
	}
	if conf != 0 {
		t.Errorf("expected 0 confidence for empty string, got %.2f", conf)
	}
}

func TestDetectInfoReliable(t *testing.T) {
	info := DetectInfo("The quick brown fox jumps over the lazy dog. This is a well-known English pangram that contains every letter of the alphabet.")
	if !info.Reliable {
		t.Errorf("expected reliable for clear English, got conf %.2f", info.Confidence)
	}
	if info.Name != "English" {
		t.Errorf("expected name English, got %s", info.Name)
	}
}

func TestDetectWithWhitelist(t *testing.T) {
	// Spanish text with whitelist that excludes Spanish → should not return es.
	info := DetectWith("Hola mundo, ¿cómo estás?", Options{
		Whitelist: []string{"en", "ru"},
	})
	if info.Lang == LangES {
		t.Errorf("Spanish should not be detected with en/ru whitelist, got %s", info.Lang)
	}
}

func TestDetectWithWhitelistEnRuZh(t *testing.T) {
	// Russian text with en/ru/zh whitelist → should return ru.
	info := DetectWith("Привет, мир! Как дела?", Options{
		Whitelist: []string{"en", "ru", "zh"},
	})
	if info.Lang != LangRU {
		t.Errorf("expected ru with whitelist, got %s (conf %.2f)", info.Lang, info.Confidence)
	}
}

func TestIsReliable(t *testing.T) {
	if !IsReliable("The quick brown fox jumps over the lazy dog. This is a well-known English pangram that contains every letter of the alphabet.") {
		t.Error("expected reliable for clear English")
	}
	if IsReliable("") {
		t.Error("expected not reliable for empty string")
	}
}

func TestCodesToLangs(t *testing.T) {
	// ISO 639-1 codes.
	m := codesToLangs([]string{"en", "ru", "zh"})
	if !m[whatlanggo.Eng] {
		t.Error("expected English in whitelist")
	}
	if !m[whatlanggo.Rus] {
		t.Error("expected Russian in whitelist")
	}
	if !m[whatlanggo.Cmn] {
		t.Error("expected Mandarin in whitelist")
	}
}

func TestIso6391To6393(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"en", "eng"},
		{"ru", "rus"},
		{"zh", "cmn"},
		{"es", "spa"},
		{"ja", "jpn"},
		{"xx", ""},
	}
	for _, tt := range tests {
		got := iso6391To6393(tt.in)
		if got != tt.want {
			t.Errorf("iso6391To6393(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
