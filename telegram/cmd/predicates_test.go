package cmd_test

import (
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/cmd"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func privMsg(userID int64) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: tgbotapi.Chat{Type: "private"},
			From: &tgbotapi.User{ID: userID},
			Text: "hello",
		},
	}
}

func groupMsg(userID int64) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: tgbotapi.Chat{Type: "group"},
			From: &tgbotapi.User{ID: userID},
			Text: "hello",
		},
	}
}

func channelPost(text string) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: tgbotapi.Chat{Type: "channel"},
			Text: text,
		},
	}
}

func msgWithPhoto() *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat:  tgbotapi.Chat{Type: "private"},
			Photo: []tgbotapi.PhotoSize{{FileID: "abc"}},
		},
	}
}

func msgWithVoice() *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat:  tgbotapi.Chat{Type: "private"},
			Voice: &tgbotapi.Voice{FileID: "abc"},
		},
	}
}

// ─── PrivateChat ─────────────────────────────────────────────────────────────

func TestPrivateChat_MatchesPrivate(t *testing.T) {
	p := cmd.PrivateChat()
	if !p(privMsg(1)) {
		t.Fatal("PrivateChat() should match private chat")
	}
}

func TestPrivateChat_RejectsGroup(t *testing.T) {
	p := cmd.PrivateChat()
	if p(groupMsg(1)) {
		t.Fatal("PrivateChat() should not match group chat")
	}
}

func TestPrivateChat_RejectsChannel(t *testing.T) {
	p := cmd.PrivateChat()
	if p(channelPost("hi")) {
		t.Fatal("PrivateChat() should not match channel")
	}
}

func TestPrivateChat_NilMessage(t *testing.T) {
	p := cmd.PrivateChat()
	if p(&tgbotapi.Update{}) {
		t.Fatal("PrivateChat() should return false for nil Message")
	}
}

// ─── GroupChat ───────────────────────────────────────────────────────────────

func TestGroupChat_MatchesGroup(t *testing.T) {
	p := cmd.GroupChat()
	if !p(groupMsg(1)) {
		t.Fatal("GroupChat() should match group chat")
	}
}

func TestGroupChat_RejectsPrivate(t *testing.T) {
	p := cmd.GroupChat()
	if p(privMsg(1)) {
		t.Fatal("GroupChat() should not match private chat")
	}
}

func TestGroupChat_MatchesSupergroup(t *testing.T) {
	upd := &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat: tgbotapi.Chat{Type: "supergroup"},
			Text: "hi",
		},
	}
	p := cmd.GroupChat()
	if !p(upd) {
		t.Fatal("GroupChat() should match supergroup")
	}
}

// ─── ChannelChat ─────────────────────────────────────────────────────────────

func TestChannelChat_MatchesChannel(t *testing.T) {
	p := cmd.ChannelChat()
	if !p(channelPost("hi")) {
		t.Fatal("ChannelChat() should match channel")
	}
}

func TestChannelChat_RejectsPrivate(t *testing.T) {
	p := cmd.ChannelChat()
	if p(privMsg(1)) {
		t.Fatal("ChannelChat() should not match private")
	}
}

// ─── FromUser ────────────────────────────────────────────────────────────────

func TestFromUser_Match(t *testing.T) {
	p := cmd.FromUser(42)
	if !p(privMsg(42)) {
		t.Fatal("FromUser(42) should match user 42")
	}
}

func TestFromUser_NoMatch(t *testing.T) {
	p := cmd.FromUser(42)
	if p(privMsg(99)) {
		t.Fatal("FromUser(42) should not match user 99")
	}
}

func TestFromUser_NilFrom(t *testing.T) {
	p := cmd.FromUser(42)
	upd := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: tgbotapi.Chat{Type: "channel"}},
	}
	if p(upd) {
		t.Fatal("FromUser should return false when Message.From is nil")
	}
}

// ─── FromAnyUser ─────────────────────────────────────────────────────────────

func TestFromAnyUser_OneMatches(t *testing.T) {
	p := cmd.FromAnyUser(10, 20, 30)
	if !p(privMsg(20)) {
		t.Fatal("FromAnyUser should match when one user ID matches")
	}
}

func TestFromAnyUser_NoneMatch(t *testing.T) {
	p := cmd.FromAnyUser(10, 20)
	if p(privMsg(99)) {
		t.Fatal("FromAnyUser should not match when no ID matches")
	}
}

func TestFromAnyUser_Empty(t *testing.T) {
	p := cmd.FromAnyUser()
	if p(privMsg(1)) {
		t.Fatal("FromAnyUser() with no IDs should always return false")
	}
}

// ─── RegexMatch ──────────────────────────────────────────────────────────────

func TestRegexMatch_Pattern(t *testing.T) {
	p := cmd.RegexMatch(`^hello`)
	upd := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: tgbotapi.Chat{Type: "private"}, Text: "hello world"},
	}
	if !p(upd) {
		t.Fatal("RegexMatch should match 'hello world' with pattern '^hello'")
	}
}

func TestRegexMatch_NoMatch(t *testing.T) {
	p := cmd.RegexMatch(`^hello`)
	upd := &tgbotapi.Update{
		Message: &tgbotapi.Message{Chat: tgbotapi.Chat{Type: "private"}, Text: "goodbye"},
	}
	if p(upd) {
		t.Fatal("RegexMatch should not match 'goodbye' with pattern '^hello'")
	}
}

func TestRegexMatch_NilMessage(t *testing.T) {
	p := cmd.RegexMatch(`.*`)
	if p(&tgbotapi.Update{}) {
		t.Fatal("RegexMatch should return false for nil Message")
	}
}

func TestRegexMatch_InvalidPattern_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid regex pattern")
		}
	}()
	cmd.RegexMatch(`[invalid`)
}

// ─── Has ─────────────────────────────────────────────────────────────────────

func TestHas_Photo(t *testing.T) {
	p := cmd.Has("Photo")
	if !p(msgWithPhoto()) {
		t.Fatal("Has('Photo') should match update with photo")
	}
}

func TestHas_Photo_NoPhoto(t *testing.T) {
	p := cmd.Has("Photo")
	if p(privMsg(1)) {
		t.Fatal("Has('Photo') should not match update without photo")
	}
}

func TestHas_Voice(t *testing.T) {
	p := cmd.Has("Voice")
	if !p(msgWithVoice()) {
		t.Fatal("Has('Voice') should match update with voice")
	}
}

func TestHas_Document_NoDocument(t *testing.T) {
	p := cmd.Has("Document")
	if p(privMsg(1)) {
		t.Fatal("Has('Document') should not match update without document")
	}
}

func TestHas_NilMessage(t *testing.T) {
	p := cmd.Has("Photo")
	if p(&tgbotapi.Update{}) {
		t.Fatal("Has should return false for nil Message")
	}
}

func TestHas_UnknownField_ReturnsFalse(t *testing.T) {
	p := cmd.Has("NonExistentField")
	if p(privMsg(1)) {
		t.Fatal("Has with unknown field should return false")
	}
}

// ─── And ─────────────────────────────────────────────────────────────────────

func TestAnd_AllTrue_True(t *testing.T) {
	p := cmd.And(cmd.PrivateChat(), cmd.FromUser(42))
	if !p(privMsg(42)) {
		t.Fatal("And should return true when all predicates pass")
	}
}

func TestAnd_OneFalse_False(t *testing.T) {
	p := cmd.And(cmd.PrivateChat(), cmd.FromUser(42))
	if p(privMsg(99)) {
		t.Fatal("And should return false when one predicate fails")
	}
}

func TestAnd_Empty_True(t *testing.T) {
	p := cmd.And()
	if !p(privMsg(1)) {
		t.Fatal("And() with no predicates should return true (vacuous)")
	}
}

// ─── Or ──────────────────────────────────────────────────────────────────────

func TestOr_OneTrue_True(t *testing.T) {
	p := cmd.Or(cmd.PrivateChat(), cmd.GroupChat())
	if !p(privMsg(1)) {
		t.Fatal("Or should return true when one predicate passes")
	}
}

func TestOr_AllFalse_False(t *testing.T) {
	p := cmd.Or(cmd.GroupChat(), cmd.ChannelChat())
	if p(privMsg(1)) {
		t.Fatal("Or should return false when all predicates fail")
	}
}

func TestOr_Empty_False(t *testing.T) {
	p := cmd.Or()
	if p(privMsg(1)) {
		t.Fatal("Or() with no predicates should return false (vacuous)")
	}
}

// ─── Not ─────────────────────────────────────────────────────────────────────

func TestNot_FlipsPredicate(t *testing.T) {
	p := cmd.Not(cmd.PrivateChat())
	if p(privMsg(1)) {
		t.Fatal("Not(PrivateChat()) should return false for private chat")
	}
	if !p(groupMsg(1)) {
		t.Fatal("Not(PrivateChat()) should return true for group chat")
	}
}

// ─── InTopic ─────────────────────────────────────────────────────────────────

func topicMsg(threadID int) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{
			Chat:            tgbotapi.Chat{Type: "supergroup"},
			MessageThreadID: threadID,
		},
	}
}

func TestInTopic_Match(t *testing.T) {
	p := cmd.InTopic(42)
	if !p(topicMsg(42)) {
		t.Fatal("InTopic(42): should match update with MessageThreadID=42")
	}
}

func TestInTopic_NoMatch(t *testing.T) {
	p := cmd.InTopic(42)
	if p(topicMsg(99)) {
		t.Fatal("InTopic(42): should not match update with MessageThreadID=99")
	}
}

func TestInTopic_Zero_MatchesNonTopicMessages(t *testing.T) {
	// InTopic(0) matches messages without a topic (MessageThreadID==0).
	// This is a deliberate design: it's the caller's responsibility to
	// use AnyTopic() or InTopic(>0) for topic-scoped filtering.
	p := cmd.InTopic(0)
	if !p(topicMsg(0)) {
		t.Fatal("InTopic(0): should match update with MessageThreadID=0")
	}
	if p(topicMsg(5)) {
		t.Fatal("InTopic(0): should not match update with MessageThreadID=5")
	}
}

func TestInTopic_NilMessage(t *testing.T) {
	p := cmd.InTopic(42)
	if p(&tgbotapi.Update{}) {
		t.Fatal("InTopic: should return false for update with nil Message")
	}
}

// ─── AnyTopic ────────────────────────────────────────────────────────────────

func TestAnyTopic_MatchPositiveThreadID(t *testing.T) {
	p := cmd.AnyTopic()
	if !p(topicMsg(1)) {
		t.Fatal("AnyTopic: should match MessageThreadID=1")
	}
	if !p(topicMsg(99)) {
		t.Fatal("AnyTopic: should match MessageThreadID=99")
	}
}

func TestAnyTopic_NoMatchZeroThreadID(t *testing.T) {
	p := cmd.AnyTopic()
	if p(topicMsg(0)) {
		t.Fatal("AnyTopic: should not match MessageThreadID=0 (general/no topic)")
	}
}

func TestAnyTopic_NilMessage(t *testing.T) {
	p := cmd.AnyTopic()
	if p(&tgbotapi.Update{}) {
		t.Fatal("AnyTopic: should return false for update with nil Message")
	}
}
