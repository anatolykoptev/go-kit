package tgapi5

import (
	"context"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Reactor wraps *tgbotapi.BotAPI and provides reaction management methods.
// DeleteReaction and DeleteAllReactions are implemented as SetMessageReaction
// with an empty reaction slice per the Telegram Bot API semantics for clearing reactions.
type Reactor struct {
	bot *tgbotapi.BotAPI
}

// NewReactor returns a *Reactor backed by bot.
func NewReactor(bot *tgbotapi.BotAPI) *Reactor {
	return &Reactor{bot: bot}
}

// SetReaction sets the reactions on a message. emojis are converted to
// []ReactionType with Type="emoji". Pass isBig=true to display a big reaction.
// An empty emojis slice clears all reactions from the bot user.
func (r *Reactor) SetReaction(ctx context.Context, chatID int64, messageID int, emojis []string, isBig bool) error {
	reactions := make([]tgbotapi.ReactionType, len(emojis))
	for i, e := range emojis {
		reactions[i] = tgbotapi.ReactionType{
			Type:  tgbotapi.ReactionTypeEmoji,
			Emoji: e,
		}
	}
	cfg := tgbotapi.SetMessageReactionConfig{
		BaseChatMessage: tgbotapi.BaseChatMessage{
			ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
			MessageID:  messageID,
		},
		Reaction: reactions,
		IsBig:    isBig,
	}
	_, err := r.bot.Request(cfg)
	return err
}

// DeleteReaction removes the bot's reaction from a message by sending an
// empty reaction set (Telegram semantics: empty slice = clear reactions).
func (r *Reactor) DeleteReaction(ctx context.Context, chatID int64, messageID int) error {
	return r.SetReaction(ctx, chatID, messageID, nil, false)
}

// DeleteAllReactions removes all bot reactions from a message. Implemented
// identically to DeleteReaction (empty reaction slice, IsBig=false).
// Note: tgbotapi also exposes DeleteAllMessageReactionsConfig for the
// deleteAllMessageReactions endpoint; this implementation uses the simpler
// setMessageReaction empty-slice approach per spec §2.3.
func (r *Reactor) DeleteAllReactions(ctx context.Context, chatID int64, messageID int) error {
	return r.SetReaction(ctx, chatID, messageID, nil, false)
}
