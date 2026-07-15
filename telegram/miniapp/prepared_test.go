package miniapp

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// fakePreparedSender implements PreparedSender for testing.
type fakePreparedSender struct {
	callCount  int
	lastUserID int64
	lastResult tgbotapi.InlineQueryResult
	lastOpts   PreparedOptions
	returnMsg  tgbotapi.PreparedInlineMessage
	returnErr  error
}

func (f *fakePreparedSender) SavePreparedInlineMessage(
	ctx context.Context,
	userID int64,
	result tgbotapi.InlineQueryResult,
	opts PreparedOptions,
) (tgbotapi.PreparedInlineMessage, error) {
	f.callCount++
	f.lastUserID = userID
	f.lastResult = result
	f.lastOpts = opts
	return f.returnMsg, f.returnErr
}

// --- SavePrepared tests ---

func TestSavePrepared_GoldenPath(t *testing.T) {
	want := tgbotapi.PreparedInlineMessage{ID: "prep_abc", ExpirationDate: 1234567890}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}
	opts := PreparedOptions{AllowUserChats: true}
	fs := &fakePreparedSender{returnMsg: want}

	got, err := SavePrepared(context.Background(), fs, 42, result, opts)
	if err != nil {
		t.Fatalf("SavePrepared golden: unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("SavePrepared returned %+v; want %+v", got, want)
	}
	if fs.callCount != 1 {
		t.Errorf("sender called %d times; want 1", fs.callCount)
	}
	if fs.lastUserID != 42 {
		t.Errorf("lastUserID = %d; want 42", fs.lastUserID)
	}
	if fs.lastOpts != opts {
		t.Errorf("lastOpts = %+v; want %+v", fs.lastOpts, opts)
	}
}

func TestSavePrepared_ErrInvalidUserID_Zero(t *testing.T) {
	fs := &fakePreparedSender{}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}

	_, err := SavePrepared(context.Background(), fs, 0, result, PreparedOptions{AllowUserChats: true})
	if !errors.Is(err, ErrInvalidUserID) {
		t.Errorf("userID=0: got %v; want ErrInvalidUserID", err)
	}
	if fs.callCount != 0 {
		t.Error("sender must not be called on validation failure")
	}
}

func TestSavePrepared_ErrInvalidUserID_Negative(t *testing.T) {
	fs := &fakePreparedSender{}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}

	_, err := SavePrepared(context.Background(), fs, -1, result, PreparedOptions{AllowUserChats: true})
	if !errors.Is(err, ErrInvalidUserID) {
		t.Errorf("userID=-1: got %v; want ErrInvalidUserID", err)
	}
	if fs.callCount != 0 {
		t.Error("sender must not be called on validation failure")
	}
}

func TestSavePrepared_ErrInvalidResult_Nil(t *testing.T) {
	fs := &fakePreparedSender{}

	_, err := SavePrepared(context.Background(), fs, 42, nil, PreparedOptions{AllowUserChats: true})
	if !errors.Is(err, ErrInvalidResult) {
		t.Errorf("result=nil: got %v; want ErrInvalidResult", err)
	}
	if fs.callCount != 0 {
		t.Error("sender must not be called on validation failure")
	}
}

func TestSavePrepared_ErrNoChatTypeAllowed(t *testing.T) {
	fs := &fakePreparedSender{}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}
	opts := PreparedOptions{} // all false

	_, err := SavePrepared(context.Background(), fs, 42, result, opts)
	if !errors.Is(err, ErrNoChatTypeAllowed) {
		t.Errorf("all flags false: got %v; want ErrNoChatTypeAllowed", err)
	}
	if fs.callCount != 0 {
		t.Error("sender must not be called on validation failure")
	}
}

func TestSavePrepared_OnlyAllowChannelChats_Passes(t *testing.T) {
	want := tgbotapi.PreparedInlineMessage{ID: "ch_msg", ExpirationDate: 9999}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}
	opts := PreparedOptions{AllowChannelChats: true}
	fs := &fakePreparedSender{returnMsg: want}

	got, err := SavePrepared(context.Background(), fs, 42, result, opts)
	if err != nil {
		t.Fatalf("AllowChannelChats only: unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("returned %+v; want %+v", got, want)
	}
	if fs.callCount != 1 {
		t.Errorf("sender called %d times; want 1", fs.callCount)
	}
}

func TestSavePrepared_SenderReturnsError(t *testing.T) {
	senderErr := errors.New("telegram: internal server error")
	fs := &fakePreparedSender{returnErr: senderErr}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}

	_, err := SavePrepared(context.Background(), fs, 42, result, PreparedOptions{AllowUserChats: true})
	if !errors.Is(err, senderErr) {
		t.Errorf("sender error: got %v; want %v", err, senderErr)
	}
	if fs.callCount != 1 {
		t.Errorf("sender called %d times; want 1", fs.callCount)
	}
}

// ctxAwarePreparedSender implements PreparedSender by reading ctx.Err itself
// inside SavePreparedInlineMessage. It proves SavePrepared actually propagates
// the cancelled context, rather than relying on a pre-baked error in the fake.
type ctxAwarePreparedSender struct {
	gotCtxErr   error
	callCount   int
	returnMsg   tgbotapi.PreparedInlineMessage
	returnErrFn func(ctx context.Context) error
}

func (f *ctxAwarePreparedSender) SavePreparedInlineMessage(
	ctx context.Context,
	userID int64,
	result tgbotapi.InlineQueryResult,
	opts PreparedOptions,
) (tgbotapi.PreparedInlineMessage, error) {
	f.callCount++
	f.gotCtxErr = ctx.Err()
	if f.returnErrFn != nil {
		return f.returnMsg, f.returnErrFn(ctx)
	}
	return f.returnMsg, nil
}

func TestSavePrepared_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	fs := &ctxAwarePreparedSender{
		returnErrFn: func(ctx context.Context) error { return ctx.Err() },
	}
	result := tgbotapi.InlineQueryResultArticle{Type: "article", ID: "1", Title: "Hi"}

	_, err := SavePrepared(ctx, fs, 42, result, PreparedOptions{AllowUserChats: true})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v; want context.Canceled", err)
	}
	// Critical assertion: the cancelled ctx must have been observed by the sender,
	// not synthesised by the fake. This proves SavePrepared propagates the ctx
	// instead of dropping it on the floor.
	if fs.callCount != 1 {
		t.Fatalf("sender called %d times; want 1", fs.callCount)
	}
	if !errors.Is(fs.gotCtxErr, context.Canceled) {
		t.Errorf("sender saw ctx.Err() = %v; want context.Canceled (proves propagation)", fs.gotCtxErr)
	}
}
