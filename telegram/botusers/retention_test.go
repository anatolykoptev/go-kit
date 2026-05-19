package botusers_test

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
	"github.com/anatolykoptev/go-kit/telegram/botusers/botuserstest"
)

func TestRetentionSweeper_RunCancels(t *testing.T) {
	store := botuserstest.NewMemStore()
	sweeper := botusers.NewRetentionSweeper(store,
		botusers.WithBotID("bot1"),
		botusers.WithSweepInterval(10*time.Millisecond),
	)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()

	// Cancel after a short time; Run must return promptly.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK — Run returned on ctx cancel.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RetentionSweeper.Run did not return after ctx cancel within 500ms")
	}
}

func TestRetentionSweeper_DeletesCalled(t *testing.T) {
	store := botuserstest.NewMemStore()
	const botID = "botSweep"

	// Insert a user.
	user := botusers.TelegramUser{TgID: 1}
	obs := botusers.Observation{At: time.Now()}
	if err := store.UpsertFromInitData(context.Background(), botID, user, obs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Run sweeper with 0 inactivity window so the user gets deleted.
	// We call Run synchronously with a very short interval and immediate cancel.
	sweeper := botusers.NewRetentionSweeper(store,
		botusers.WithBotID(botID),
		botusers.WithSweepInterval(5*time.Millisecond),
		botusers.WithInactivityWindow(0), // delete everyone
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	sweeper.Run(ctx) // synchronous; returns when ctx expires

	// User should be gone.
	_, err := store.Get(context.Background(), botID, user.TgID)
	if err == nil {
		t.Fatal("expected ErrNotFound after sweep, got nil error")
	}
	if !isNotFoundErr(err) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func isNotFoundErr(err error) bool {
	for err != nil {
		if err == botusers.ErrNotFound {
			return true
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
