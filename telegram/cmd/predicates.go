package cmd

import (
	"reflect"
	"regexp"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Predicate is a function that inspects a Telegram Update and returns true
// if the Update satisfies the predicate's condition.
// Predicates compose with And, Or, and Not; Routes accept them via When.
type Predicate func(*tgbotapi.Update) bool

// PrivateChat returns a Predicate that passes only for private-chat messages.
func PrivateChat() Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		return upd.Message.Chat.Type == "private"
	}
}

// GroupChat returns a Predicate that passes for group and supergroup messages.
func GroupChat() Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		t := upd.Message.Chat.Type
		return t == "group" || t == "supergroup"
	}
}

// ChannelChat returns a Predicate that passes for channel posts.
func ChannelChat() Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		return upd.Message.Chat.Type == "channel"
	}
}

// FromUser returns a Predicate that passes only when the message sender's ID
// matches userID. Returns false when Message or Message.From is nil.
func FromUser(userID int64) Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil || upd.Message.From == nil {
			return false
		}
		return upd.Message.From.ID == userID
	}
}

// FromAnyUser returns a Predicate that passes when the sender's ID is one of
// the provided userIDs. Returns false for an empty list or a nil sender.
func FromAnyUser(userIDs ...int64) Predicate {
	set := make(map[int64]struct{}, len(userIDs))
	for _, id := range userIDs {
		set[id] = struct{}{}
	}
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil || upd.Message.From == nil {
			return false
		}
		_, ok := set[upd.Message.From.ID]
		return ok
	}
}

// RegexMatch returns a Predicate that passes when Message.Text matches pattern.
// Panics at construction time if pattern is not a valid regular expression.
func RegexMatch(pattern string) Predicate {
	re := regexp.MustCompile(pattern)
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		return re.MatchString(upd.Message.Text)
	}
}

// Has returns a Predicate that passes when the named field of Message is
// non-zero (non-nil pointer, non-empty slice, or non-zero value).
// Field is matched by exported struct field name (e.g. "Photo", "Voice", "Document").
// Returns false when Message is nil or the field name does not exist.
func Has(field string) Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		rv := reflect.ValueOf(*upd.Message)
		fv := rv.FieldByName(field)
		if !fv.IsValid() {
			return false
		}
		switch fv.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Chan, reflect.Func:
			return !fv.IsNil()
		case reflect.Slice:
			return fv.Len() > 0
		default:
			return !fv.IsZero()
		}
	}
}

// And returns a Predicate that passes only when all of the given predicates pass.
// With no arguments, And returns a predicate that always passes (vacuous truth).
func And(p ...Predicate) Predicate {
	return func(upd *tgbotapi.Update) bool {
		for _, pred := range p {
			if !pred(upd) {
				return false
			}
		}
		return true
	}
}

// Or returns a Predicate that passes when at least one of the given predicates passes.
// With no arguments, Or returns a predicate that always fails (vacuous falsity).
func Or(p ...Predicate) Predicate {
	return func(upd *tgbotapi.Update) bool {
		for _, pred := range p {
			if pred(upd) {
				return true
			}
		}
		return false
	}
}

// Not returns a Predicate that inverts the result of p.
func Not(p Predicate) Predicate {
	return func(upd *tgbotapi.Update) bool {
		return !p(upd)
	}
}

// InTopic returns a Predicate that passes when the message's MessageThreadID
// equals id. Callers should pass a positive id for a specific topic.
// Passing id=0 matches messages that are NOT in any topic (the general chat
// stream); prefer AnyTopic() for filtering any non-general topic.
// Returns false when Message is nil.
func InTopic(id int) Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		return upd.Message.MessageThreadID == id
	}
}

// AnyTopic returns a Predicate that passes when the message originates from
// any non-general forum topic (MessageThreadID > 0).
// Returns false when Message is nil.
func AnyTopic() Predicate {
	return func(upd *tgbotapi.Update) bool {
		if upd.Message == nil {
			return false
		}
		return upd.Message.MessageThreadID > 0
	}
}

// OnReaction returns a Predicate that passes when the Update contains a
// message_reaction event (Update.MessageReaction != nil).
//
// If emojis is empty, any reaction update matches. If emojis are provided,
// the predicate passes only when at least one emoji in Update.MessageReaction.NewReaction
// intersects with the provided list. Non-emoji reaction types (custom_emoji, paid)
// are ignored during intersection.
func OnReaction(emojis ...string) Predicate {
	filter := make(map[string]struct{}, len(emojis))
	for _, e := range emojis {
		filter[e] = struct{}{}
	}
	return func(upd *tgbotapi.Update) bool {
		if upd.MessageReaction == nil {
			return false
		}
		if len(filter) == 0 {
			return true
		}
		for _, rt := range upd.MessageReaction.NewReaction {
			if rt.Type == tgbotapi.ReactionTypeEmoji {
				if _, ok := filter[rt.Emoji]; ok {
					return true
				}
			}
		}
		return false
	}
}
