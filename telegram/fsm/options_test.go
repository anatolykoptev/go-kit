package fsm_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/fsm"
)

// TestWithCancelCmds_CancelsSession verifies that when an incoming text matches
// one of the registered cancel commands, the current session is deleted and the
// configured OnCancel hook is invoked.
//
// Spec §3.D / PR-5 task body: WithCancelCmds + WithOnCancel.
func TestWithCancelCmds_CancelsSession(t *testing.T) {
	store := fsm.NewMemoryStore()

	var cancelCalled atomic.Int32
	var cancelledChatID atomic.Int64

	loopFn := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		var self fsm.StateFn
		self = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
			return self, nil // loop forever
		}
		return self, nil
	}

	m := fsm.New(
		store,
		func(flow string) fsm.StateFn { return loopFn },
		time.Hour,
		fsm.WithCancelCmds("/cancel", "stop"),
		fsm.WithOnCancel(func(ctx context.Context, chatID int64) {
			cancelCalled.Add(1)
			cancelledChatID.Store(chatID)
		}),
	)

	ctx := context.Background()
	if err := m.Start(ctx, 1001, "flow"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Feed a non-cancel message — session should stay.
	handled, err := m.Feed(ctx, makeEvent(1001, "hello"))
	if err != nil {
		t.Fatalf("Feed hello: %v", err)
	}
	if !handled {
		t.Fatal("Feed hello: want handled=true")
	}

	// Feed the cancel command — session should be deleted and hook called.
	handled, err = m.Feed(ctx, makeEvent(1001, "/cancel"))
	if err != nil {
		t.Fatalf("Feed /cancel: %v", err)
	}
	if !handled {
		t.Fatal("Feed /cancel: want handled=true (cancel command handled)")
	}

	// Hook must have fired.
	if cancelCalled.Load() != 1 {
		t.Fatalf("OnCancel not called: got %d calls", cancelCalled.Load())
	}
	if cancelledChatID.Load() != 1001 {
		t.Fatalf("OnCancel wrong chatID: got %d want 1001", cancelledChatID.Load())
	}

	// Session must be gone — next Feed returns handled=false.
	handled, err = m.Feed(ctx, makeEvent(1001, "hi again"))
	if err != nil {
		t.Fatalf("Feed after cancel: %v", err)
	}
	if handled {
		t.Fatal("Feed after cancel: want handled=false (session deleted)")
	}
}

// TestWithCancelCmds_AltCmd verifies that a second cancel command also works.
func TestWithCancelCmds_AltCmd(t *testing.T) {
	store := fsm.NewMemoryStore()

	var hookFired atomic.Bool

	loopFn := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		var self fsm.StateFn
		self = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return self, nil }
		return self, nil
	}

	m := fsm.New(
		store,
		func(flow string) fsm.StateFn { return loopFn },
		time.Hour,
		fsm.WithCancelCmds("/cancel", "stop"),
		fsm.WithOnCancel(func(ctx context.Context, chatID int64) {
			hookFired.Store(true)
		}),
	)

	ctx := context.Background()
	if err := m.Start(ctx, 1002, "flow"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	handled, err := m.Feed(ctx, makeEvent(1002, "stop"))
	if err != nil {
		t.Fatalf("Feed stop: %v", err)
	}
	if !handled {
		t.Fatal("Feed stop: want handled=true")
	}
	if !hookFired.Load() {
		t.Fatal("OnCancel hook not fired for alt cancel command")
	}
}

// TestWithCancelCmds_NoHookIsOK verifies WithCancelCmds without WithOnCancel
// still cancels the session — hook is optional.
func TestWithCancelCmds_NoHookIsOK(t *testing.T) {
	store := fsm.NewMemoryStore()

	loopFn := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		var self fsm.StateFn
		self = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return self, nil }
		return self, nil
	}

	m := fsm.New(
		store,
		func(flow string) fsm.StateFn { return loopFn },
		time.Hour,
		fsm.WithCancelCmds("/cancel"),
		// no WithOnCancel
	)

	ctx := context.Background()
	if err := m.Start(ctx, 1003, "flow"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	handled, err := m.Feed(ctx, makeEvent(1003, "/cancel"))
	if err != nil {
		t.Fatalf("Feed /cancel: %v", err)
	}
	if !handled {
		t.Fatal("Feed /cancel: want handled=true")
	}

	// Session must be gone.
	handled, _ = m.Feed(ctx, makeEvent(1003, "next"))
	if handled {
		t.Fatal("After cancel-no-hook: session should be gone")
	}
}
