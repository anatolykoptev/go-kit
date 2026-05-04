package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestFullLifecycle_InMemory — полный цикл: сообщения → компактификация → факты → очистка.
func TestFullLifecycle_InMemory(t *testing.T) {
	store := NewInMemoryStore(Options{MaxMessages: 50})

	// 1. Добавляем диалог.
	store.AddMessage("user1", Message{Role: "user", Content: "Настрой nginx на порт 8080"})
	store.AddMessage("user1", Message{Role: "assistant", Content: "Готово, nginx слушает 8080"})
	store.AddMessage("user1", Message{Role: "user", Content: "А теперь добавь SSL"})
	store.AddMessage("user1", Message{Role: "assistant", Content: "SSL настроен через certbot"})

	if store.MessageCount("user1") != 4 {
		t.Fatalf("expected 4 messages, got %d", store.MessageCount("user1"))
	}

	// 2. История возвращает копию — мутация не влияет на оригинал.
	history := store.GetHistory("user1")
	history[0].Content = "MUTATED"
	original := store.GetHistory("user1")
	if original[0].Content == "MUTATED" {
		t.Fatal("GetHistory should return a copy")
	}

	// 3. Компактификация с извлечением фактов.
	for range 20 {
		store.AddMessage("user1", Message{Role: "user", Content: "ещё сообщение про деплой"})
	}

	compactor := Compactor{
		Store:        store,
		Threshold:    15,
		KeepLast:     5,
		ExtractFacts: true,
		Summarize: func(_ context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "Summarize") && !strings.Contains(prompt, "Extract") {
				t.Error("prompt should contain summarization instruction")
			}
			return "- nginx настроен на порт 8080\n- SSL через certbot\n- деплой обсуждался", nil
		},
	}

	compactor.Compact(context.Background(), "user1")

	// После компактификации: 5 сообщений, 3 факта.
	if store.MessageCount("user1") != 5 {
		t.Errorf("after compact: %d messages, want 5", store.MessageCount("user1"))
	}
	facts := store.GetFacts("user1")
	if len(facts) != 3 {
		t.Fatalf("expected 3 facts, got %d", len(facts))
	}
	if facts[0].Content != "nginx настроен на порт 8080" {
		t.Errorf("fact[0] = %q", facts[0].Content)
	}

	// 4. Факты накапливаются при повторной компактификации.
	for range 20 {
		store.AddMessage("user1", Message{Role: "user", Content: "обсуждаем мониторинг"})
	}
	compactor.Summarize = func(_ context.Context, _ string) (string, error) {
		return "- добавлен мониторинг через prometheus", nil
	}
	compactor.Compact(context.Background(), "user1")

	facts = store.GetFacts("user1")
	if len(facts) != 4 {
		t.Errorf("facts should accumulate: got %d, want 4", len(facts))
	}
}

// TestFullLifecycle_FileStore — файловое хранилище сохраняет и восстанавливает всё.
func TestFullLifecycle_FileStore(t *testing.T) {
	dir := t.TempDir()

	// Создаём сессию с сообщениями, саммари и фактами.
	s1 := NewFileStore(dir, Options{})
	s1.AddMessage("chat42", Message{Role: "user", Content: "важный вопрос"})
	s1.AddMessage("chat42", Message{Role: "assistant", Content: "важный ответ"})
	s1.SetSummary("chat42", "обсуждали важную тему")
	s1.AddFacts("chat42", []Fact{
		{Content: "сервер на Ubuntu", ExtractedAt: time.Now()},
		{Content: "порт 443", ExtractedAt: time.Now()},
	})

	if err := s1.Save("chat42"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Загружаем в новый store — всё должно восстановиться.
	s2 := NewFileStore(dir, Options{})

	msgs := s2.GetHistory("chat42")
	if len(msgs) != 2 || msgs[0].Content != "важный вопрос" {
		t.Errorf("messages not restored: %+v", msgs)
	}
	if s2.GetSummary("chat42") != "обсуждали важную тему" {
		t.Errorf("summary not restored: %q", s2.GetSummary("chat42"))
	}
	facts := s2.GetFacts("chat42")
	if len(facts) != 2 || facts[1].Content != "порт 443" {
		t.Errorf("facts not restored: %+v", facts)
	}
}

// TestFullLifecycle_Cleanup — очистка старых сессий с архивацией.
func TestFullLifecycle_Cleanup(t *testing.T) {
	store := NewInMemoryStore(Options{})

	// Активная сессия.
	store.AddMessage("active", Message{Role: "user", Content: "работаю"})

	// Старая сессия (2 дня назад).
	store.AddMessage("stale", Message{Role: "user", Content: "забытый диалог"})
	store.SetSummary("stale", "старый разговор")
	stale := store.GetOrCreate("stale")
	stale.Updated = time.Now().Add(-48 * time.Hour)

	// Архивная функция собирает данные перед удалением.
	var archived []string
	archiveFn := func(s *Session) error {
		archived = append(archived, fmt.Sprintf("key=%s msgs=%d summary=%q",
			s.Key, len(s.Messages), s.Summary))
		return nil
	}

	cleaned := Cleanup(store, 24*time.Hour, archiveFn)

	if cleaned != 1 {
		t.Errorf("cleaned %d, want 1", cleaned)
	}
	if len(archived) != 1 {
		t.Fatalf("archived %d sessions, want 1", len(archived))
	}
	if !strings.Contains(archived[0], "key=stale") {
		t.Errorf("wrong session archived: %s", archived[0])
	}
	if !strings.Contains(archived[0], `summary="старый разговор"`) {
		t.Errorf("archive should contain summary: %s", archived[0])
	}

	// Активная сессия осталась.
	if store.GetHistory("active") == nil {
		t.Error("active session should survive cleanup")
	}
	// Старая удалена.
	if store.GetHistory("stale") != nil {
		t.Error("stale session should be deleted")
	}
}

// TestIntentFilter_RealWorldPhrases — проверка на реальных фразах.
func TestIntentFilter_RealWorldPhrases(t *testing.T) {
	skip := []string{
		"Привет!",
		"Привет, как дела?",
		"Спасибо!",
		"Thanks a lot",
		"Thank you for helping",
		"До свидания",
		"До встречи!",
		"Доброе утро!",
		"Hello there",
		"ok",
		"да",
		"bye",
	}
	for _, s := range skip {
		if NeedsMemoryContext(s) {
			t.Errorf("should skip %q", s)
		}
	}

	need := []string{
		"Помнишь, мы обсуждали архитектуру?",
		"Покажи задачи на сегодня",
		"Напиши функцию для парсинга JSON",
		"Как устроен роутер?",
		"recall the task list",
		"почему не работает nginx",
		"/status",
		"Привет, помнишь что мы делали?", // force overrides skip
	}
	for _, s := range need {
		if !NeedsMemoryContext(s) {
			t.Errorf("should need memory for %q", s)
		}
	}
}
