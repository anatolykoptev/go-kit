package cmd_test

import (
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/cmd"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func reactionUpdate(emojis ...string) *tgbotapi.Update {
	reactions := make([]tgbotapi.ReactionType, len(emojis))
	for i, e := range emojis {
		reactions[i] = tgbotapi.ReactionType{
			Type:  tgbotapi.ReactionTypeEmoji,
			Emoji: e,
		}
	}
	return &tgbotapi.Update{
		MessageReaction: &tgbotapi.MessageReactionUpdated{
			Chat:        tgbotapi.Chat{ID: 100},
			MessageID:   1,
			NewReaction: reactions,
		},
	}
}

// ─── OnReaction ──────────────────────────────────────────────────────────────

// TestOnReaction_EmptyFilter matches any update with MessageReaction set.
func TestOnReaction_EmptyFilter_MatchesAny(t *testing.T) {
	p := cmd.OnReaction()
	if !p(reactionUpdate("👍")) {
		t.Fatal("OnReaction() with no emojis should match any reaction update")
	}
}

// TestOnReaction_EmptyFilter_NoReaction returns false when Update.MessageReaction is nil.
func TestOnReaction_EmptyFilter_NilMessageReaction(t *testing.T) {
	p := cmd.OnReaction()
	if p(&tgbotapi.Update{}) {
		t.Fatal("OnReaction() should return false when MessageReaction is nil")
	}
}

// TestOnReaction_EmojiMatch passes when at least one emoji intersects.
func TestOnReaction_EmojiIntersect_Match(t *testing.T) {
	p := cmd.OnReaction("👍", "❤️")
	if !p(reactionUpdate("🔥", "👍")) {
		t.Fatal("OnReaction should match when emoji intersects")
	}
}

// TestOnReaction_EmojiIntersect_NoMatch returns false when no emoji intersects.
func TestOnReaction_EmojiIntersect_NoMatch(t *testing.T) {
	p := cmd.OnReaction("👍", "❤️")
	if p(reactionUpdate("🔥", "😂")) {
		t.Fatal("OnReaction should return false when no emoji intersects")
	}
}

// TestOnReaction_NilMessageReaction_WithFilter returns false for non-reaction update.
func TestOnReaction_NilMessageReaction_WithFilter(t *testing.T) {
	p := cmd.OnReaction("👍")
	if p(privMsg(42)) {
		t.Fatal("OnReaction should return false for non-reaction update")
	}
}

// TestOnReaction_EmptyNewReaction_EmptyFilter passes even if NewReaction is empty
// as long as MessageReaction is set.
func TestOnReaction_EmptyNewReaction_EmptyFilter(t *testing.T) {
	p := cmd.OnReaction()
	upd := &tgbotapi.Update{
		MessageReaction: &tgbotapi.MessageReactionUpdated{
			NewReaction: []tgbotapi.ReactionType{},
		},
	}
	if !p(upd) {
		t.Fatal("OnReaction() with no filter should match even when NewReaction is empty")
	}
}

// TestOnReaction_EmptyNewReaction_WithFilter returns false when filter given but NewReaction empty.
func TestOnReaction_EmptyNewReaction_WithFilter(t *testing.T) {
	p := cmd.OnReaction("👍")
	upd := &tgbotapi.Update{
		MessageReaction: &tgbotapi.MessageReactionUpdated{
			NewReaction: []tgbotapi.ReactionType{},
		},
	}
	if p(upd) {
		t.Fatal("OnReaction with filter should return false when NewReaction is empty")
	}
}
