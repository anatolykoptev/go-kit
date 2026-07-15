// Package forum provides a Manager for supergroup forum-topic operations.
//
// # Overview
//
// Manager wraps the Telegram Bot API forum-topic endpoints and exposes a
// clean, context-aware Go interface. All methods are safe to call from
// multiple goroutines; Manager holds no mutable state.
//
// # Usage
//
//	bot, _ := tgbotapi.NewBotAPI(token)
//	mgr    := forum.NewManager(bot)   // *tgbotapi.BotAPI satisfies forum.Sender
//
//	topic, err := mgr.Create(ctx, chatID, "Support",
//	    forum.WithIconColor(0x6FB9F0),
//	)
//	if err != nil { ... }
//
//	if err := mgr.Close(ctx, chatID, topic.MessageThreadID); err != nil { ... }
package forum

import (
	"context"
	"encoding/json"
	"fmt"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Sender is the minimal interface Manager uses to dispatch Telegram API
// requests. *tgbotapi.BotAPI satisfies this interface directly via its
// RequestWithContext method, so no adapter is needed.
type Sender interface {
	RequestWithContext(ctx context.Context, c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// Topic is the library's representation of a Telegram forum topic.
// It mirrors tgbotapi.ForumTopic but is owned by this package so callers
// do not need to import the upstream API package just to inspect a topic.
type Topic struct {
	MessageThreadID   int
	Name              string
	IconColor         int
	IconCustomEmojiID string
}

// Manager dispatches Telegram forum-topic API calls.
// It holds no mutable state; all operations are thread-safe.
type Manager struct {
	sender Sender
}

// NewManager returns a Manager backed by s.
func NewManager(s Sender) *Manager {
	return &Manager{sender: s}
}

// ─── Topic operations ─────────────────────────────────────────────────────────

// Create creates a new forum topic in chatID with the given name.
// Optional CreateOptions configure IconColor and IconCustomEmojiID.
// Returns the created Topic on success.
func (m *Manager) Create(ctx context.Context, chatID int64, name string, opts ...CreateOption) (*Topic, error) {
	cfg := tgbotapi.CreateForumTopicConfig{
		ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
		Name:       name,
	}
	for _, o := range opts {
		o(&cfg)
	}

	resp, err := m.sender.RequestWithContext(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("forum: create topic: %w", err)
	}

	var ft tgbotapi.ForumTopic
	if err := json.Unmarshal(resp.Result, &ft); err != nil {
		return nil, fmt.Errorf("forum: create topic: decode response: %w", err)
	}

	return &Topic{
		MessageThreadID:   ft.MessageThreadID,
		Name:              ft.Name,
		IconColor:         ft.IconColor,
		IconCustomEmojiID: ft.IconCustomEmojiID,
	}, nil
}

// Edit edits the name and/or icon of an existing forum topic.
// At least one EditOption should be provided; an empty call is a no-op on
// the Telegram side.
func (m *Manager) Edit(ctx context.Context, chatID int64, threadID int, opts ...EditOption) error {
	cfg := tgbotapi.EditForumTopicConfig{
		BaseForum: tgbotapi.BaseForum{
			ChatConfig:      tgbotapi.ChatConfig{ChatID: chatID},
			MessageThreadID: threadID,
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	return m.request(ctx, cfg)
}

// Close closes an open forum topic.
func (m *Manager) Close(ctx context.Context, chatID int64, threadID int) error {
	return m.request(ctx, tgbotapi.CloseForumTopicConfig{
		BaseForum: baseForum(chatID, threadID),
	})
}

// Reopen reopens a closed forum topic.
func (m *Manager) Reopen(ctx context.Context, chatID int64, threadID int) error {
	return m.request(ctx, tgbotapi.ReopenForumTopicConfig{
		BaseForum: baseForum(chatID, threadID),
	})
}

// Delete deletes a forum topic along with all its messages.
func (m *Manager) Delete(ctx context.Context, chatID int64, threadID int) error {
	return m.request(ctx, tgbotapi.DeleteForumTopicConfig{
		BaseForum: baseForum(chatID, threadID),
	})
}

// UnpinAll clears the list of pinned messages in a forum topic.
func (m *Manager) UnpinAll(ctx context.Context, chatID int64, threadID int) error {
	return m.request(ctx, tgbotapi.UnpinAllForumTopicMessagesConfig{
		BaseForum: baseForum(chatID, threadID),
	})
}

// ─── General-topic variants ───────────────────────────────────────────────────

// EditGeneral edits the name of the 'General' forum topic.
func (m *Manager) EditGeneral(ctx context.Context, chatID int64, name string) error {
	return m.request(ctx, tgbotapi.EditGeneralForumTopicConfig{
		BaseForum: baseForum(chatID, 0),
		Name:      name,
	})
}

// CloseGeneral closes the 'General' forum topic.
func (m *Manager) CloseGeneral(ctx context.Context, chatID int64) error {
	return m.request(ctx, tgbotapi.CloseGeneralForumTopicConfig{
		BaseForum: baseForum(chatID, 0),
	})
}

// ReopenGeneral reopens the 'General' forum topic.
func (m *Manager) ReopenGeneral(ctx context.Context, chatID int64) error {
	return m.request(ctx, tgbotapi.ReopenGeneralForumTopicConfig{
		BaseForum: baseForum(chatID, 0),
	})
}

// HideGeneral hides the 'General' forum topic.
func (m *Manager) HideGeneral(ctx context.Context, chatID int64) error {
	return m.request(ctx, tgbotapi.HideGeneralForumTopicConfig{
		BaseForum: baseForum(chatID, 0),
	})
}

// UnhideGeneral unhides the 'General' forum topic.
func (m *Manager) UnhideGeneral(ctx context.Context, chatID int64) error {
	return m.request(ctx, tgbotapi.UnhideGeneralForumTopicConfig{
		BaseForum: baseForum(chatID, 0),
	})
}

// UnpinAllGeneral clears the list of pinned messages in the 'General' topic.
func (m *Manager) UnpinAllGeneral(ctx context.Context, chatID int64) error {
	return m.request(ctx, tgbotapi.UnpinAllGeneralForumTopicMessagesConfig{
		BaseForum: baseForum(chatID, 0),
	})
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// request dispatches c and discards the response body, returning any error.
func (m *Manager) request(ctx context.Context, c tgbotapi.Chattable) error {
	if _, err := m.sender.RequestWithContext(ctx, c); err != nil {
		return fmt.Errorf("forum: %w", err)
	}
	return nil
}

// baseForum constructs a BaseForum for the given chatID and threadID.
func baseForum(chatID int64, threadID int) tgbotapi.BaseForum {
	return tgbotapi.BaseForum{
		ChatConfig:      tgbotapi.ChatConfig{ChatID: chatID},
		MessageThreadID: threadID,
	}
}
