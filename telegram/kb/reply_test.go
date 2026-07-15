package kb

import (
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// TestReplyBuilder_BasicGrid verifies 3 rows × 2 buttons produce the expected markup shape.
func TestReplyBuilder_BasicGrid(t *testing.T) {
	b := NewReply().
		Text("A").Text("B").
		Row().
		Text("C").Text("D").
		Row().
		Text("E").Text("F")

	m := b.Markup()

	if len(m.Keyboard) != 3 {
		t.Fatalf("want 3 rows, got %d", len(m.Keyboard))
	}
	for i, row := range m.Keyboard {
		if len(row) != 2 {
			t.Errorf("row %d: want 2 buttons, got %d", i, len(row))
		}
	}
	if m.Keyboard[0][0].Text != "A" || m.Keyboard[0][1].Text != "B" {
		t.Errorf("row 0 labels mismatch: got %q, %q", m.Keyboard[0][0].Text, m.Keyboard[0][1].Text)
	}
	if m.Keyboard[2][0].Text != "E" || m.Keyboard[2][1].Text != "F" {
		t.Errorf("row 2 labels mismatch: got %q, %q", m.Keyboard[2][0].Text, m.Keyboard[2][1].Text)
	}
}

// TestReplyBuilder_Resize_Persistent_OneTime_Placeholder verifies option flags propagate.
func TestReplyBuilder_Resize_Persistent_OneTime_Placeholder(t *testing.T) {
	b := NewReply(
		ReplyResize(),
		ReplyPersistent(),
		ReplyOneTime(),
		ReplyPlaceholder("Type here"),
		ReplySelective(),
	).Text("OK")

	m := b.Markup()

	if !m.ResizeKeyboard {
		t.Error("want ResizeKeyboard=true")
	}
	if !m.IsPersistent {
		t.Error("want IsPersistent=true")
	}
	if !m.OneTimeKeyboard {
		t.Error("want OneTimeKeyboard=true")
	}
	if m.InputFieldPlaceholder != "Type here" {
		t.Errorf("want placeholder %q, got %q", "Type here", m.InputFieldPlaceholder)
	}
	if !m.Selective {
		t.Error("want Selective=true")
	}
}

// TestReplyBuilder_RequestContact verifies RequestContact=true is set on the button.
func TestReplyBuilder_RequestContact(t *testing.T) {
	b := NewReply().RequestContact("Share phone")

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button, got shape %v", shapeOf(m.Keyboard))
	}
	btn := m.Keyboard[0][0]
	if btn.Text != "Share phone" {
		t.Errorf("want label %q, got %q", "Share phone", btn.Text)
	}
	if !btn.RequestContact {
		t.Error("want RequestContact=true")
	}
}

// TestReplyBuilder_RequestLocation verifies RequestLocation=true is set on the button.
func TestReplyBuilder_RequestLocation(t *testing.T) {
	b := NewReply().RequestLocation("Share location")

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button, got shape %v", shapeOf(m.Keyboard))
	}
	btn := m.Keyboard[0][0]
	if btn.Text != "Share location" {
		t.Errorf("want label %q, got %q", "Share location", btn.Text)
	}
	if !btn.RequestLocation {
		t.Error("want RequestLocation=true")
	}
}

// TestReplyBuilder_RequestPoll verifies RequestPoll is set with the correct poll type.
func TestReplyBuilder_RequestPoll(t *testing.T) {
	b := NewReply().RequestPoll("Create quiz", "quiz")

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button, got shape %v", shapeOf(m.Keyboard))
	}
	btn := m.Keyboard[0][0]
	if btn.RequestPoll == nil {
		t.Fatal("want RequestPoll set, got nil")
	}
	if btn.RequestPoll.Type != "quiz" {
		t.Errorf("want poll type %q, got %q", "quiz", btn.RequestPoll.Type)
	}
}

// TestReplyBuilder_WebApp verifies the button emits KeyboardButton with WebApp.URL set.
func TestReplyBuilder_WebApp(t *testing.T) {
	const appURL = "https://example.com/app"
	b := NewReply().WebApp("Open App", appURL)

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button, got shape %v", shapeOf(m.Keyboard))
	}
	btn := m.Keyboard[0][0]
	if btn.Text != "Open App" {
		t.Errorf("want label %q, got %q", "Open App", btn.Text)
	}
	if btn.WebApp == nil {
		t.Fatal("want WebApp set, got nil")
	}
	if btn.WebApp.URL != appURL {
		t.Errorf("want WebApp.URL %q, got %q", appURL, btn.WebApp.URL)
	}
}

// TestReplyBuilder_RequestUser verifies RequestUsers is forwarded to the button.
func TestReplyBuilder_RequestUser(t *testing.T) {
	req := tgbotapi.KeyboardButtonRequestUsers{RequestID: 42}
	b := NewReply().RequestUser("Pick user", req)

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button")
	}
	btn := m.Keyboard[0][0]
	if btn.RequestUsers == nil {
		t.Fatal("want RequestUsers set, got nil")
	}
	if btn.RequestUsers.RequestID != 42 {
		t.Errorf("want RequestID=42, got %d", btn.RequestUsers.RequestID)
	}
}

// TestReplyBuilder_RequestChat verifies RequestChat is forwarded to the button.
func TestReplyBuilder_RequestChat(t *testing.T) {
	req := tgbotapi.KeyboardButtonRequestChat{RequestID: 7, ChatIsChannel: true}
	b := NewReply().RequestChat("Pick chat", req)

	m := b.Markup()

	if len(m.Keyboard) != 1 || len(m.Keyboard[0]) != 1 {
		t.Fatalf("want 1 row × 1 button")
	}
	btn := m.Keyboard[0][0]
	if btn.RequestChat == nil {
		t.Fatal("want RequestChat set, got nil")
	}
	if btn.RequestChat.RequestID != 7 || !btn.RequestChat.ChatIsChannel {
		t.Errorf("unexpected RequestChat value: %+v", btn.RequestChat)
	}
}

// TestRemoveReply_TrueFalse verifies ReplyKeyboardRemove is emitted correctly.
func TestRemoveReply_TrueFalse(t *testing.T) {
	r1 := RemoveReply(false)
	if !r1.RemoveKeyboard {
		t.Error("want RemoveKeyboard=true")
	}
	if r1.Selective {
		t.Error("want Selective=false when selective=false")
	}

	r2 := RemoveReply(true)
	if !r2.RemoveKeyboard {
		t.Error("want RemoveKeyboard=true")
	}
	if !r2.Selective {
		t.Error("want Selective=true when selective=true")
	}
}

// shapeOf returns row lengths for error messages.
func shapeOf(rows [][]tgbotapi.KeyboardButton) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = len(r)
	}
	return out
}
