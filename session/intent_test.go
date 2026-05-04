package session

import "testing"

func TestNeedsMemoryContext(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"empty", "", false},
		{"short", "hi", false},
		{"greeting_ru", "привет", false},
		{"greeting_en", "hello", false},
		{"greeting_hey", "hey", false},
		{"thanks_ru", "спасибо", false},
		{"thanks_en", "thanks", false},
		{"ok_ru", "ок", false},
		{"yes_no", "да", false},
		{"bye_ru", "пока", false},
		{"bye_en", "bye", false},
		{"greeting_with_space", "привет как дела", false},
		{"substantive_ru", "почему не работает nginx", true},
		{"substantive_en", "why is the server down", true},
		{"force_remember_ru", "помнишь что мы обсуждали?", true},
		{"force_remember_en", "do you remember the config?", true},
		{"force_recall", "recall what we discussed", true},
		{"force_запомни", "запомни что порт 8080", true},
		{"greeting_but_long", "привет я хочу обсудить проблему с сервером и деплоем на продакшн", true},
		{"unicode_emoji", "🚀 deploy the app", true},
		{"mixed_force_skip", "привет помнишь наш разговор?", true},
		{"command", "/status", true},
		{"numbers", "12345", true},
		{"greeting_ru_excl", "Привет!", false},
		{"greeting_ru_comma", "Привет, как дела?", false},
		{"greeting_morning_ru", "Доброе утро!", false},
		{"thanks_ru_excl", "Спасибо!", false},
		{"thanks_en_lot", "Thanks a lot", false},
		{"thank_you_en", "Thank you for helping", false},
		{"farewell_ru", "до свидания", false},
		{"farewell_ru_excl", "До встречи!", false},
		{"hello_there", "Hello there", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsMemoryContext(tt.text)
			if got != tt.want {
				t.Errorf("NeedsMemoryContext(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
