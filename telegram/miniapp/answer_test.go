package miniapp

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// fakeAnswerer implements WebAppAnswerer for testing.
// It records the last call and returns a fixed response or error.
type fakeAnswerer struct {
	lastQueryID string
	lastResult  tgbotapi.InlineQueryResult
	returnMsg   *tgbotapi.SentWebAppMessage
	returnErr   error
}

func (f *fakeAnswerer) AnswerWebAppQuery(
	_ context.Context,
	queryID string,
	result tgbotapi.InlineQueryResult,
) (*tgbotapi.SentWebAppMessage, error) {
	f.lastQueryID = queryID
	f.lastResult = result
	return f.returnMsg, f.returnErr
}

func TestReply_GoldenPath(t *testing.T) {
	want := &tgbotapi.SentWebAppMessage{InlineMessageID: "msg-42"}
	fa := &fakeAnswerer{returnMsg: want}
	result := tgbotapi.InlineQueryResultArticle{
		Type:  "article",
		ID:    "1",
		Title: "Hello",
	}

	got, err := Reply(context.Background(), fa, "qid-1", result)
	if err != nil {
		t.Fatalf("Reply returned unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("Reply returned %+v; want %+v", got, want)
	}
	if fa.lastQueryID != "qid-1" {
		t.Errorf("forwarded queryID = %q; want %q", fa.lastQueryID, "qid-1")
	}
	if fa.lastResult != result {
		t.Errorf("forwarded result = %+v; want %+v", fa.lastResult, result)
	}
}

func TestReply_PropagatesAnswererError(t *testing.T) {
	wantErr := errors.New("bot error")
	fa := &fakeAnswerer{returnErr: wantErr}

	_, err := Reply(context.Background(), fa, "qid-2", tgbotapi.InlineQueryResultArticle{})
	if !errors.Is(err, wantErr) {
		t.Errorf("Reply error = %v; want %v", err, wantErr)
	}
}
