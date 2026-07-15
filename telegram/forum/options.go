package forum

import tgbotapi "github.com/OvyFlash/telegram-bot-api"

// CreateOption is a functional option for Manager.Create.
// Options are applied left to right; later options override earlier ones.
type CreateOption func(*tgbotapi.CreateForumTopicConfig)

// WithIconColor sets the color of the forum topic icon (RGB format).
// Telegram accepts a fixed set of values; see the Telegram Bot API docs.
func WithIconColor(c int) CreateOption {
	return func(cfg *tgbotapi.CreateForumTopicConfig) {
		cfg.IconColor = c
	}
}

// WithIconCustomEmoji sets a custom emoji ID as the topic icon.
// Pass an empty string to clear a previously set emoji.
func WithIconCustomEmoji(emojiID string) CreateOption {
	return func(cfg *tgbotapi.CreateForumTopicConfig) {
		cfg.IconCustomEmojiID = emojiID
	}
}

// EditOption is a functional option for Manager.Edit.
// Options are applied left to right; later options override earlier ones.
// Note: only the topic name and custom emoji ID are editable after creation;
// icon color is fixed at creation time and cannot be changed via editForumTopic.
type EditOption func(*tgbotapi.EditForumTopicConfig)

// WithName sets a new display name for the forum topic.
func WithName(name string) EditOption {
	return func(cfg *tgbotapi.EditForumTopicConfig) {
		cfg.Name = name
	}
}

// WithEmoji sets a new custom emoji ID for the forum topic icon.
// Pass an empty string to remove the icon and fall back to the icon color.
func WithEmoji(emojiID string) EditOption {
	return func(cfg *tgbotapi.EditForumTopicConfig) {
		cfg.IconCustomEmojiID = emojiID
	}
}
