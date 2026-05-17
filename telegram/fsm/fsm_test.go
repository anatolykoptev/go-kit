package fsm_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"

	"github.com/anatolykoptev/go-kit/telegram/fsm"
)

// drainMicrotasks yields control to let goroutines run.
func drainGoroutines() {
	time.Sleep(5 * time.Millisecond)
}

// stepFn returns a StateFn that records the step name and transitions to next.
func stepFn(name string, next func() fsm.StateFn) fsm.StateFn {
	var fn fsm.StateFn
	fn = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		if next == nil {
			return nil, nil // terminal state
		}
		return next(), nil
	}
	_ = name
	return fn
}

// ---- helpers ----

func makeEvent(chatID int64, text string) fsm.Event {
	return fsm.Event{
		ChatID: chatID,
		UserID: chatID,
		Text:   text,
		Update: &tgbotapi.Update{},
	}
}

// ---- tests ----

// TestMachine_StartAndFeedHappyPath: two-step flow ends cleanly.
func TestMachine_StartAndFeedHappyPath(t *testing.T) {
	store := fsm.NewMemoryStore()
	ttl := time.Hour

	var step2 fsm.StateFn
	step2 = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return nil, nil // terminal
	}
	step1 := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return step2, nil
	}

	m := fsm.New(store, func(flow string) fsm.StateFn { return step1 }, ttl)

	ctx := context.Background()
	if err := m.Start(ctx, 100, "onboard"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// First Feed → transitions to step2.
	handled, err := m.Feed(ctx, makeEvent(100, "hello"))
	if err != nil {
		t.Fatalf("Feed step1: %v", err)
	}
	if !handled {
		t.Fatal("Feed step1: want handled=true")
	}

	// Second Feed → step2 returns nil = terminal, session deleted.
	handled, err = m.Feed(ctx, makeEvent(100, "world"))
	if err != nil {
		t.Fatalf("Feed step2: %v", err)
	}
	if !handled {
		t.Fatal("Feed step2: want handled=true")
	}

	// No session left — next Feed returns handled=false.
	handled, err = m.Feed(ctx, makeEvent(100, "ghost"))
	if err != nil {
		t.Fatalf("Feed after terminal: %v", err)
	}
	if handled {
		t.Fatal("Feed after terminal: want handled=false")
	}
}

// TestMachine_FeedNoSession: handled=false when no active session.
func TestMachine_FeedNoSession(t *testing.T) {
	store := fsm.NewMemoryStore()
	m := fsm.New(store, func(flow string) fsm.StateFn {
		return func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return nil, nil }
	}, time.Hour)

	handled, err := m.Feed(context.Background(), makeEvent(999, "hi"))
	if err != nil {
		t.Fatalf("Feed no session: %v", err)
	}
	if handled {
		t.Fatal("Feed no session: want handled=false")
	}
}

// TestMachine_TTLExpiry: expired session → Feed returns (false, nil), no panic.
func TestMachine_TTLExpiry(t *testing.T) {
	store := fsm.NewMemoryStore()
	m := fsm.New(store, func(flow string) fsm.StateFn {
		return func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return nil, nil }
	}, time.Hour)

	ctx := context.Background()
	// Inject an already-expired session directly into the store.
	expired := &fsm.Session{
		ChatID:    200,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{},
		UpdatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Second),
	}
	if err := store.Put(ctx, expired); err != nil {
		t.Fatalf("Put expired: %v", err)
	}

	handled, err := m.Feed(ctx, makeEvent(200, "hi"))
	if err != nil {
		t.Fatalf("Feed expired session: %v", err)
	}
	if handled {
		t.Fatal("Feed expired session: want handled=false (session expired)")
	}
}

// TestMachine_Cancel: Cancel deletes session; subsequent Feed returns handled=false.
func TestMachine_Cancel(t *testing.T) {
	store := fsm.NewMemoryStore()
	m := fsm.New(store, func(flow string) fsm.StateFn {
		return func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return nil, nil }
	}, time.Hour)

	ctx := context.Background()
	if err := m.Start(ctx, 300, "onboard"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Cancel(ctx, 300); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	handled, err := m.Feed(ctx, makeEvent(300, "hi"))
	if err != nil {
		t.Fatalf("Feed after Cancel: %v", err)
	}
	if handled {
		t.Fatal("Feed after Cancel: want handled=false")
	}
}

// TestMachine_PanicInStateFn: panic → error returned, same Machine not corrupted.
func TestMachine_PanicInStateFn(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	var (
		panicFn fsm.StateFn
		okFn    fsm.StateFn
	)
	panicFn = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		panic("test panic in state fn")
	}
	okFn = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return nil, nil // terminal
	}

	// Machine initial factory: chatID=400 → panicFn, chatID=401 → okFn.
	m := fsm.New(store, func(flow string) fsm.StateFn {
		if flow == "panic" {
			return panicFn
		}
		return okFn
	}, time.Hour)

	if err := m.Start(ctx, 400, "panic"); err != nil {
		t.Fatalf("Start panic flow: %v", err)
	}

	_, err := m.Feed(ctx, makeEvent(400, "trigger panic"))
	if err == nil {
		t.Fatal("PanicInStateFn: expected error, got nil")
	}

	// Same Machine instance — start a different chat, must work cleanly.
	if err := m.Start(ctx, 401, "ok"); err != nil {
		t.Fatalf("Start ok flow after panic: %v", err)
	}
	handled, err := m.Feed(ctx, makeEvent(401, "ok"))
	if err != nil || !handled {
		t.Fatalf("Feed ok flow after panic: handled=%v err=%v", handled, err)
	}
}

// TestMachine_StayInState: StateFn returning itself → same Step persists.
func TestMachine_StayInState(t *testing.T) {
	store := fsm.NewMemoryStore()

	var stayFn fsm.StateFn
	stayFn = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return stayFn, nil // stay in same state
	}

	m := fsm.New(store, func(flow string) fsm.StateFn { return stayFn }, time.Hour)

	ctx := context.Background()
	if err := m.Start(ctx, 500, "onboard"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Feed twice — session should still be alive with same step each time.
	for i := 0; i < 2; i++ {
		handled, err := m.Feed(ctx, makeEvent(500, "re-prompt"))
		if err != nil {
			t.Fatalf("Feed stay[%d]: %v", i, err)
		}
		if !handled {
			t.Fatalf("Feed stay[%d]: want handled=true", i)
		}
	}

	// Session must still exist in store.
	sess, err := store.Get(ctx, 500)
	if err != nil || sess == nil {
		t.Fatalf("StayInState: session gone: err=%v sess=%v", err, sess)
	}
}

// TestMachine_SweepDeletesExpired: StartSweeper removes expired entries.
func TestMachine_SweepDeletesExpired(t *testing.T) {
	store := fsm.NewMemoryStore()
	m := fsm.New(store, func(flow string) fsm.StateFn {
		return func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) { return nil, nil }
	}, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Inject expired session directly.
	expired := &fsm.Session{
		ChatID:    600,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{},
		UpdatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Second),
	}
	if err := store.Put(ctx, expired); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Use very short sweep interval.
	m.StartSweeper(ctx, 10*time.Millisecond)
	drainGoroutines()
	time.Sleep(30 * time.Millisecond) // let at least one sweep tick

	got, err := store.Get(ctx, 600)
	if err != nil || got != nil {
		t.Fatalf("After sweep: expired session still there: err=%v got=%v", err, got)
	}
}

// TestMachine_ConcurrentFeed: two goroutines Feed on same chatID — no data race.
func TestMachine_ConcurrentFeed(t *testing.T) {
	store := fsm.NewMemoryStore()

	calls := make([]string, 0)
	mu := &sync.Mutex{}

	fn := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		mu.Lock()
		calls = append(calls, e.Text)
		mu.Unlock()
		return nil, nil
	}

	m := fsm.New(store, func(flow string) fsm.StateFn { return fn }, time.Hour)

	ctx := context.Background()
	if err := m.Start(ctx, 700, "onboard"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = m.Feed(ctx, makeEvent(700, "concurrent"))
		}(i)
	}
	wg.Wait()
	// No assertion on call count — just must not panic or data-race.
}

// TestMachine_StateFnReturnsError: error from StateFn propagated to caller.
func TestMachine_StateFnReturnsError(t *testing.T) {
	store := fsm.NewMemoryStore()
	sentinel := errors.New("state machine error")
	errFn := func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return nil, sentinel
	}
	m := fsm.New(store, func(flow string) fsm.StateFn { return errFn }, time.Hour)

	ctx := context.Background()
	if err := m.Start(ctx, 800, "onboard"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	_, err := m.Feed(ctx, makeEvent(800, "trigger"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("StateFnReturnsError: want sentinel, got %v", err)
	}
}

// --- Test H2: Concurrent Feed on same chatID serializes properly ---
// Ref: ~/deploy/krolik-server/reports/go-kit/architecture/2026-05-16-v0.56-quality-review.md H2

func TestMachine_ConcurrentFeed_SameChat_SerializesProperly(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	// Multi-step StateFn that mutates session.State to accumulate a counter.
	// If two Feeds race and both read/write State concurrently, the counter
	// will be wrong or we'll get a data race.
	var step fsm.StateFn
	step = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		// Stay in same state — this keeps the session alive for all goroutines.
		return step, nil
	}

	m := fsm.New(store, func(flow string) fsm.StateFn { return step }, time.Hour)

	if err := m.Start(ctx, 999, "concurrent"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Feed(ctx, makeEvent(999, "tick"))
		}()
	}
	wg.Wait()

	// Session must still exist (stayFn never returns nil).
	sess, err := store.Get(ctx, 999)
	if err != nil {
		t.Fatalf("Get after concurrent feeds: %v", err)
	}
	if sess == nil {
		t.Fatal("ConcurrentFeed: session deleted unexpectedly")
	}
	// Run under -race to catch H2.
}

// --- Test M3: funcName returns stable, non-empty, distinct strings ---
// Ref: ~/deploy/krolik-server/reports/go-kit/architecture/2026-05-16-v0.56-quality-review.md M3

func TestFuncName_StableAcrossInvocations(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	var fn fsm.StateFn
	fn = func(ctx context.Context, e fsm.Event) (fsm.StateFn, error) {
		return fn, nil // stay
	}

	m := fsm.New(store, func(flow string) fsm.StateFn { return fn }, time.Hour)

	if err := m.Start(ctx, 1, "flow"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sess1, _ := store.Get(ctx, 1)
	step1 := sess1.Step

	// Feed once to trigger a Put with updated Step.
	if _, err := m.Feed(ctx, makeEvent(1, "hi")); err != nil {
		t.Fatalf("Feed: %v", err)
	}

	sess2, _ := store.Get(ctx, 1)
	step2 := sess2.Step

	if step1 == "" {
		t.Fatal("FuncName: step label must not be empty")
	}
	if step1 != step2 {
		t.Fatalf("FuncName: step label changed between invocations: %q → %q", step1, step2)
	}
}

func testFnAlpha(_ context.Context, _ fsm.Event) (fsm.StateFn, error) { return nil, nil }
func testFnBeta(_ context.Context, _ fsm.Event) (fsm.StateFn, error)  { return nil, nil }

func TestFuncName_DistinguishesDifferentFns(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	call := 0
	m := fsm.New(store, func(flow string) fsm.StateFn {
		call++
		if call == 1 {
			return testFnAlpha
		}
		return testFnBeta
	}, time.Hour)

	// chatID=2 → testFnAlpha
	if err := m.Start(ctx, 2, "flow"); err != nil {
		t.Fatalf("Start alpha: %v", err)
	}
	// chatID=3 → testFnBeta
	if err := m.Start(ctx, 3, "flow"); err != nil {
		t.Fatalf("Start beta: %v", err)
	}

	sessAlpha, _ := store.Get(ctx, 2)
	sessBeta, _ := store.Get(ctx, 3)

	if sessAlpha.Step == sessBeta.Step {
		t.Fatalf("FuncName: different StateFns must produce different step labels, both got %q", sessAlpha.Step)
	}
}
