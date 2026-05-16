package middleware

// Sender is the interface for sending a plain-text Telegram message to a chat.
// The tgapi5 subpackage provides a default implementation backed by *tgbotapi.BotAPI.
//
// Consumers that need richer send semantics (markup, parse mode, force-reply)
// should compose or type-assert to an extended interface.
type Sender interface {
	Send(chatID int64, text string) error
}

// CallbackAnswerer is the interface for answering Telegram callback queries.
// Every callback must be answered within 10 seconds so the loading spinner
// clears. The tgapi5 subpackage provides a default implementation.
type CallbackAnswerer interface {
	AnswerCallback(callbackID string) error
}

// MessageDeleter is the interface for deleting a Telegram message by chat+message ID.
// Implementations are expected to classify "too old to delete" errors distinctly
// from other failures; see tgapi5.MetricDeleteTotal label documentation.
type MessageDeleter interface {
	DeleteMessage(chatID int64, messageID int) error
}
