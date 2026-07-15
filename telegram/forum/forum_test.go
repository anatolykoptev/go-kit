package forum_test

import (
	"context"
	"encoding/json"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/forum"
)

// ─── fake Sender ─────────────────────────────────────────────────────────────

// fakeSender records the most recent Chattable dispatched and returns a
// configurable response. It is not goroutine-safe; tests are single-threaded.
type fakeSender struct {
	last    tgbotapi.Chattable
	resp    *tgbotapi.APIResponse
	respErr error
}

func (f *fakeSender) RequestWithContext(_ context.Context, c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	f.last = c
	if f.resp != nil {
		return f.resp, f.respErr
	}
	return &tgbotapi.APIResponse{Ok: true}, f.respErr
}

// okResp returns an *APIResponse whose Result field contains the JSON encoding
// of v. Used to satisfy Manager.Create's unmarshal of resp.Result.
func okResp(t *testing.T, v any) *tgbotapi.APIResponse {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("okResp: json.Marshal: %v", err)
	}
	return &tgbotapi.APIResponse{Ok: true, Result: raw}
}

// ─── Manager.Create ──────────────────────────────────────────────────────────

func TestCreate_Basic(t *testing.T) {
	ft := tgbotapi.ForumTopic{
		MessageThreadID:   42,
		Name:              "Support",
		IconColor:         0x6FB9F0,
		IconCustomEmojiID: "",
	}
	s := &fakeSender{resp: okResp(t, ft)}
	m := forum.NewManager(s)

	topic, err := m.Create(context.Background(), 100, "Support")
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if topic.MessageThreadID != 42 {
		t.Errorf("MessageThreadID: got %d, want 42", topic.MessageThreadID)
	}
	if topic.Name != "Support" {
		t.Errorf("Name: got %q, want %q", topic.Name, "Support")
	}

	cfg, ok := s.last.(tgbotapi.CreateForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want CreateForumTopicConfig", s.last)
	}
	if cfg.ChatConfig.ChatID != 100 {
		t.Errorf("ChatID: got %d, want 100", cfg.ChatConfig.ChatID)
	}
	if cfg.Name != "Support" {
		t.Errorf("cfg.Name: got %q, want %q", cfg.Name, "Support")
	}
}

func TestCreate_WithOptions(t *testing.T) {
	ft := tgbotapi.ForumTopic{MessageThreadID: 7, Name: "VIP"}
	s := &fakeSender{resp: okResp(t, ft)}
	m := forum.NewManager(s)

	_, err := m.Create(context.Background(), 200, "VIP",
		forum.WithIconColor(0xFF0000),
		forum.WithIconCustomEmoji("emoji_id_123"),
	)
	if err != nil {
		t.Fatalf("Create with options: %v", err)
	}

	cfg, ok := s.last.(tgbotapi.CreateForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want CreateForumTopicConfig", s.last)
	}
	if cfg.IconColor != 0xFF0000 {
		t.Errorf("IconColor: got %#x, want %#x", cfg.IconColor, 0xFF0000)
	}
	if cfg.IconCustomEmojiID != "emoji_id_123" {
		t.Errorf("IconCustomEmojiID: got %q, want %q", cfg.IconCustomEmojiID, "emoji_id_123")
	}
}

func TestCreate_SenderError(t *testing.T) {
	s := &fakeSender{respErr: errFake}
	m := forum.NewManager(s)

	_, err := m.Create(context.Background(), 1, "X")
	if err == nil {
		t.Fatal("expected error from Create when sender fails, got nil")
	}
}

var errFake = &fakeErr{"sender failure"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

// ─── Manager.Edit ────────────────────────────────────────────────────────────

func TestEdit_Basic(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.Edit(context.Background(), 100, 42); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	cfg, ok := s.last.(tgbotapi.EditForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want EditForumTopicConfig", s.last)
	}
	if cfg.BaseForum.ChatConfig.ChatID != 100 {
		t.Errorf("ChatID: got %d, want 100", cfg.BaseForum.ChatConfig.ChatID)
	}
	if cfg.BaseForum.MessageThreadID != 42 {
		t.Errorf("MessageThreadID: got %d, want 42", cfg.BaseForum.MessageThreadID)
	}
}

func TestEdit_WithOptions(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.Edit(context.Background(), 100, 42,
		forum.WithName("New Name"),
		forum.WithEmoji("em_id"),
	); err != nil {
		t.Fatalf("Edit with options: %v", err)
	}

	cfg, ok := s.last.(tgbotapi.EditForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want EditForumTopicConfig", s.last)
	}
	if cfg.Name != "New Name" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "New Name")
	}
	if cfg.IconCustomEmojiID != "em_id" {
		t.Errorf("IconCustomEmojiID: got %q, want em_id", cfg.IconCustomEmojiID)
	}
}

// ─── Manager.Close / Reopen / Delete / UnpinAll ──────────────────────────────

func TestClose(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.Close(context.Background(), 100, 42); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cfg, ok := s.last.(tgbotapi.CloseForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want CloseForumTopicConfig", s.last)
	}
	assertBaseForum(t, cfg.BaseForum, 100, 42)
}

func TestReopen(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.Reopen(context.Background(), 100, 42); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	cfg, ok := s.last.(tgbotapi.ReopenForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want ReopenForumTopicConfig", s.last)
	}
	assertBaseForum(t, cfg.BaseForum, 100, 42)
}

func TestDelete(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.Delete(context.Background(), 100, 42); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	cfg, ok := s.last.(tgbotapi.DeleteForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want DeleteForumTopicConfig", s.last)
	}
	assertBaseForum(t, cfg.BaseForum, 100, 42)
}

func TestUnpinAll(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.UnpinAll(context.Background(), 100, 42); err != nil {
		t.Fatalf("UnpinAll: %v", err)
	}
	cfg, ok := s.last.(tgbotapi.UnpinAllForumTopicMessagesConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want UnpinAllForumTopicMessagesConfig", s.last)
	}
	assertBaseForum(t, cfg.BaseForum, 100, 42)
}

// ─── Manager.EditGeneral / CloseGeneral / ReopenGeneral / HideGeneral / UnhideGeneral / UnpinAllGeneral ──

func TestEditGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.EditGeneral(context.Background(), 100, "Main"); err != nil {
		t.Fatalf("EditGeneral: %v", err)
	}
	cfg, ok := s.last.(tgbotapi.EditGeneralForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want EditGeneralForumTopicConfig", s.last)
	}
	if cfg.Name != "Main" {
		t.Errorf("Name: got %q, want %q", cfg.Name, "Main")
	}
}

func TestCloseGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.CloseGeneral(context.Background(), 100); err != nil {
		t.Fatalf("CloseGeneral: %v", err)
	}
	_, ok := s.last.(tgbotapi.CloseGeneralForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want CloseGeneralForumTopicConfig", s.last)
	}
}

func TestReopenGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.ReopenGeneral(context.Background(), 100); err != nil {
		t.Fatalf("ReopenGeneral: %v", err)
	}
	_, ok := s.last.(tgbotapi.ReopenGeneralForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want ReopenGeneralForumTopicConfig", s.last)
	}
}

func TestHideGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.HideGeneral(context.Background(), 100); err != nil {
		t.Fatalf("HideGeneral: %v", err)
	}
	_, ok := s.last.(tgbotapi.HideGeneralForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want HideGeneralForumTopicConfig", s.last)
	}
}

func TestUnhideGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.UnhideGeneral(context.Background(), 100); err != nil {
		t.Fatalf("UnhideGeneral: %v", err)
	}
	_, ok := s.last.(tgbotapi.UnhideGeneralForumTopicConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want UnhideGeneralForumTopicConfig", s.last)
	}
}

func TestUnpinAllGeneral(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	if err := m.UnpinAllGeneral(context.Background(), 100); err != nil {
		t.Fatalf("UnpinAllGeneral: %v", err)
	}
	_, ok := s.last.(tgbotapi.UnpinAllGeneralForumTopicMessagesConfig)
	if !ok {
		t.Fatalf("Chattable type: got %T, want UnpinAllGeneralForumTopicMessagesConfig", s.last)
	}
}

// ─── Options ordering ────────────────────────────────────────────────────────

// TestCreateOptions_OrderingPreserved verifies that when two options both set
// the same field (WithIconColor applied twice), the last one wins (functional
// options are applied left to right).
func TestCreateOptions_OrderingPreserved(t *testing.T) {
	ft := tgbotapi.ForumTopic{MessageThreadID: 1, Name: "X"}
	s := &fakeSender{resp: okResp(t, ft)}
	m := forum.NewManager(s)

	_, err := m.Create(context.Background(), 1, "X",
		forum.WithIconColor(0x111111),
		forum.WithIconColor(0x222222), // last wins
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cfg := s.last.(tgbotapi.CreateForumTopicConfig)
	if cfg.IconColor != 0x222222 {
		t.Errorf("IconColor ordering: got %#x, want %#x", cfg.IconColor, 0x222222)
	}
}

// TestEditOptions_OrderingPreserved checks WithName applied twice.
func TestEditOptions_OrderingPreserved(t *testing.T) {
	s := &fakeSender{}
	m := forum.NewManager(s)

	err := m.Edit(context.Background(), 1, 1,
		forum.WithName("first"),
		forum.WithName("second"), // last wins
	)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}

	cfg := s.last.(tgbotapi.EditForumTopicConfig)
	if cfg.Name != "second" {
		t.Errorf("WithName ordering: got %q, want %q", cfg.Name, "second")
	}
}

// ─── Sender interface compile-time check ─────────────────────────────────────

// BotAPI must satisfy forum.Sender so callers don't need an adapter.
var _ forum.Sender = (*tgbotapi.BotAPI)(nil)

// ─── helpers ─────────────────────────────────────────────────────────────────

func assertBaseForum(t *testing.T, b tgbotapi.BaseForum, wantChatID int64, wantThreadID int) {
	t.Helper()
	if b.ChatConfig.ChatID != wantChatID {
		t.Errorf("ChatID: got %d, want %d", b.ChatConfig.ChatID, wantChatID)
	}
	if b.MessageThreadID != wantThreadID {
		t.Errorf("MessageThreadID: got %d, want %d", b.MessageThreadID, wantThreadID)
	}
}
