package botusers_test

import (
	"context"
	"errors"
	"sync"
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

// errStore is a Store that always returns an error from DeleteInactive.
// Used to test sweeper error handling.
type errStore struct {
	botuserstest.MemStore // embed to satisfy the full interface
	deleteErr error
}

func (e *errStore) DeleteInactive(ctx context.Context, botID string, olderThan time.Duration) (int64, error) {
	return 0, e.deleteErr
}

func TestRetentionSweeper_ErrorEmitsMetrics(t *testing.T) {
	// M4: when DeleteInactive returns an error, the sweeper must call
	// metrics.Incr("bot_users.sweep_error") and must NOT panic.
	store := &errStore{
		MemStore:  *botuserstest.NewMemStore(),
		deleteErr: errors.New("simulated db error"),
	}

	emitter := &captureEmitter{}
	sweeper := botusers.NewRetentionSweeper(store,
		botusers.WithBotID("bot1"),
		botusers.WithSweepInterval(5*time.Millisecond),
		botusers.WithMetrics(emitter),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sweeper.Run(ctx)

	// At least one sweep_error counter bump must have occurred.
	emitter.mu.Lock()
	incrs := emitter.incrs
	emitter.mu.Unlock()

	found := false
	for _, name := range incrs {
		if name == "bot_users.sweep_error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Incr(%q) to be called on sweep error; got incrs: %v",
			"bot_users.sweep_error", incrs)
	}
}

func TestRetentionSweeper_SuccessEmitsGauge(t *testing.T) {
	// M4: on successful sweep, bot_users.last_sweep_deleted gauge must be set.
	store := botuserstest.NewMemStore()
	const botID = "botGauge"

	// Insert a user that will be deleted.
	user := botusers.TelegramUser{TgID: 42}
	obs := botusers.Observation{At: time.Now()}
	if err := store.UpsertFromInitData(context.Background(), botID, user, obs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	emitter := &captureEmitter{}
	sweeper := botusers.NewRetentionSweeper(store,
		botusers.WithBotID(botID),
		botusers.WithSweepInterval(5*time.Millisecond),
		botusers.WithInactivityWindow(0), // delete all
		botusers.WithMetrics(emitter),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	sweeper.Run(ctx)

	emitter.mu.Lock()
	gauges := emitter.gauges
	emitter.mu.Unlock()

	if _, ok := gauges["bot_users.last_sweep_deleted"]; !ok {
		t.Errorf("expected Gauge(%q) to be called after sweep; gauges: %v",
			"bot_users.last_sweep_deleted", gauges)
	}
}

// captureEmitter records all Incr and Gauge calls thread-safely.
type captureEmitter struct {
	mu     sync.Mutex
	incrs  []string
	gauges map[string]float64
}

func (c *captureEmitter) Incr(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.incrs = append(c.incrs, name)
}

func (c *captureEmitter) Gauge(name string, value float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gauges == nil {
		c.gauges = make(map[string]float64)
	}
	c.gauges[name] = value
}
