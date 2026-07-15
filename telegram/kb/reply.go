package kb

import tgbotapi "github.com/OvyFlash/telegram-bot-api"

// replyConfig holds the markup-level options for a ReplyKeyboardMarkup.
type replyConfig struct {
	resize      bool
	oneTime     bool
	persistent  bool
	placeholder string
	selective   bool
}

// ReplyOption configures a ReplyBuilder.
type ReplyOption func(*replyConfig)

// ReplyResize sets resize_keyboard=true, asking clients to resize the keyboard
// vertically for an optimal fit.
func ReplyResize() ReplyOption { return func(c *replyConfig) { c.resize = true } }

// ReplyOneTime sets one_time_keyboard=true, asking clients to hide the keyboard
// after the first use.
func ReplyOneTime() ReplyOption { return func(c *replyConfig) { c.oneTime = true } }

// ReplyPersistent sets is_persistent=true (Bot API 6.5+), requesting clients to
// always show the keyboard even when the regular keyboard is hidden.
func ReplyPersistent() ReplyOption { return func(c *replyConfig) { c.persistent = true } }

// ReplyPlaceholder sets the input_field_placeholder shown in the text input area
// while the keyboard is active (1–64 characters).
func ReplyPlaceholder(text string) ReplyOption {
	return func(c *replyConfig) { c.placeholder = text }
}

// ReplySelective sets selective=true, showing the keyboard only to @mentioned
// users or the sender of the replied-to message.
func ReplySelective() ReplyOption { return func(c *replyConfig) { c.selective = true } }

// ReplyBuilder constructs a tgbotapi.ReplyKeyboardMarkup with a fluent row API.
// The zero value is not usable; construct via NewReply.
type ReplyBuilder struct {
	cfg    replyConfig
	markup [][]tgbotapi.KeyboardButton
}

// NewReply creates a ReplyBuilder with the given options applied.
func NewReply(opts ...ReplyOption) *ReplyBuilder {
	b := &ReplyBuilder{
		markup: [][]tgbotapi.KeyboardButton{{}},
	}
	for _, opt := range opts {
		opt(&b.cfg)
	}
	return b
}

// Row starts a new button row. No-op if the current row is already empty
// (mirrors the InlineKeyboard builder's behaviour).
func (b *ReplyBuilder) Row() *ReplyBuilder {
	if len(b.markup[len(b.markup)-1]) > 0 {
		b.markup = append(b.markup, []tgbotapi.KeyboardButton{})
	}
	return b
}

// Text appends a plain text button. When pressed, it sends the label as a
// message from the user.
func (b *ReplyBuilder) Text(label string) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{Text: label})
}

// RequestContact appends a button that, when pressed, sends the user's phone
// number to the bot as a contact. Available in private chats only.
func (b *ReplyBuilder) RequestContact(label string) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{Text: label, RequestContact: true})
}

// RequestLocation appends a button that, when pressed, sends the user's current
// location to the bot. Available in private chats only.
func (b *ReplyBuilder) RequestLocation(label string) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{Text: label, RequestLocation: true})
}

// RequestPoll appends a button that asks the user to create and send a poll.
// pollType must be "quiz", "regular", or "" (any type). Available in private
// chats only.
func (b *ReplyBuilder) RequestPoll(label string, pollType string) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{
		Text:        label,
		RequestPoll: &tgbotapi.KeyboardButtonPollType{Type: pollType},
	})
}

// WebApp appends a button that launches a Telegram Mini App at url.
// Available in private chats only.
func (b *ReplyBuilder) WebApp(label, url string) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{
		Text:   label,
		WebApp: &tgbotapi.WebAppInfo{URL: url},
	})
}

// RequestUser appends a button that opens a user-picker dialog. The selected
// user's ID is sent to the bot in a user_shared service message. Available in
// private chats only.
func (b *ReplyBuilder) RequestUser(label string, req tgbotapi.KeyboardButtonRequestUsers) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{
		Text:         label,
		RequestUsers: &req,
	})
}

// RequestChat appends a button that opens a chat-picker dialog. The selected
// chat's ID is sent to the bot in a chat_shared service message. Available in
// private chats only.
func (b *ReplyBuilder) RequestChat(label string, req tgbotapi.KeyboardButtonRequestChat) *ReplyBuilder {
	return b.appendButton(tgbotapi.KeyboardButton{
		Text:        label,
		RequestChat: &req,
	})
}

// Markup builds and returns the tgbotapi.ReplyKeyboardMarkup.
// Returns a deep copy of the internal row slice so callers cannot mutate
// the builder's state.
func (b *ReplyBuilder) Markup() tgbotapi.ReplyKeyboardMarkup {
	rows := make([][]tgbotapi.KeyboardButton, len(b.markup))
	for i, row := range b.markup {
		cp := make([]tgbotapi.KeyboardButton, len(row))
		copy(cp, row)
		rows[i] = cp
	}
	return tgbotapi.ReplyKeyboardMarkup{
		Keyboard:              rows,
		IsPersistent:          b.cfg.persistent,
		ResizeKeyboard:        b.cfg.resize,
		OneTimeKeyboard:       b.cfg.oneTime,
		InputFieldPlaceholder: b.cfg.placeholder,
		Selective:             b.cfg.selective,
	}
}

// appendButton adds btn to the current row.
func (b *ReplyBuilder) appendButton(btn tgbotapi.KeyboardButton) *ReplyBuilder {
	last := len(b.markup) - 1
	b.markup[last] = append(b.markup[last], btn)
	return b
}

// RemoveReply returns a tgbotapi.ReplyKeyboardRemove that instructs Telegram
// clients to remove the persistent reply keyboard. If selective is true, the
// keyboard is removed only for @mentioned users or the sender of the
// replied-to message.
func RemoveReply(selective bool) tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.ReplyKeyboardRemove{
		RemoveKeyboard: true,
		Selective:      selective,
	}
}
