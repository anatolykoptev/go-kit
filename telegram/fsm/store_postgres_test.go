package fsm_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anatolykoptev/go-kit/telegram/fsm"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DROP TABLE IF EXISTS bootstrap_bot_sessions") //nolint:errcheck
		pool.Close()
	})
	return pool
}

func TestPostgresStore_RoundTrip(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	// Get on empty store returns nil, nil.
	sess, err := store.Get(ctx, 1001)
	if err != nil || sess != nil {
		t.Fatalf("Get empty: err=%v sess=%v", err, sess)
	}

	s := &fsm.Session{
		ChatID:    1001,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{"key": "value"},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Put(ctx, s); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get(ctx, 1001)
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if got == nil {
		t.Fatal("Get after Put: got nil")
	}
	if got.Flow != "onboard" || got.Step != "ask_id" {
		t.Fatalf("Get after Put: wrong fields: %+v", got)
	}
	if got.State["key"] != "value" {
		t.Fatalf("Get after Put: wrong state: %+v", got.State)
	}
}

func TestPostgresStore_GetExpired(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	// Insert an already-expired session via raw SQL.
	_, err = pool.Exec(ctx, `
		INSERT INTO bootstrap_bot_sessions (chat_id, flow, step, state_json, updated_at, expires_at)
		VALUES ($1, 'f', 's', '{}'::jsonb, now(), now() - interval '1 second')
		ON CONFLICT (chat_id) DO UPDATE SET expires_at = EXCLUDED.expires_at
	`, int64(1002))
	if err != nil {
		t.Fatalf("Insert expired: %v", err)
	}

	got, err := store.Get(ctx, 1002)
	if err != nil || got != nil {
		t.Fatalf("Get expired: expected nil, err=%v got=%v", err, got)
	}
}

func TestPostgresStore_Delete(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	s := &fsm.Session{
		ChatID:    1003,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Put(ctx, s); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, 1003); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.Get(ctx, 1003)
	if err != nil || got != nil {
		t.Fatalf("Get after Delete: err=%v got=%v", err, got)
	}
}

func TestPostgresStore_Sweep(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	// Expired session via raw SQL.
	_, err = pool.Exec(ctx, `
		INSERT INTO bootstrap_bot_sessions (chat_id, flow, step, state_json, updated_at, expires_at)
		VALUES ($1, 'f', 's', '{}'::jsonb, now(), now() - interval '1 second')
	`, int64(1010))
	if err != nil {
		t.Fatalf("Insert expired: %v", err)
	}

	// Live session.
	if err := store.Put(ctx, &fsm.Session{
		ChatID:    1011,
		Flow:      "f",
		Step:      "s",
		State:     map[string]any{},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put alive: %v", err)
	}

	deleted, err := store.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Sweep: want 1 deleted, got %d", deleted)
	}

	got, err := store.Get(ctx, 1011)
	if err != nil || got == nil {
		t.Fatalf("Sweep: alive session gone: err=%v got=%v", err, got)
	}
}

// TestPostgresStore_Get_ContextCancelled_PropagatesError verifies that a
// cancelled context returns an error (not nil, nil).
// Ref: ~/deploy/krolik-server/reports/go-kit/architecture/2026-05-16-v0.56-quality-review.md M1
func TestPostgresStore_Get_ContextCancelled_PropagatesError(t *testing.T) {
	pool := newTestPool(t)

	store, err := fsm.NewPostgresStore(context.Background(), pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = store.Get(ctx, 9999)
	if err == nil {
		t.Fatal("Get with cancelled ctx: expected non-nil error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get with cancelled ctx: expected context.Canceled in error chain, got: %v", err)
	}
}

// bootstrapOnboardingController stands in for a real consumer's bot
// controller (e.g. oxpulse-admin/internal/bootstrap.KitBot) whose StateFns
// are bound methods. Taking a method value (not a top-level func) makes
// funcName() return the full package-import-path + receiver type + method
// name + "-fm" suffix — routinely >64 chars in a real module tree.
type bootstrapOnboardingController struct{}

func (c *bootstrapOnboardingController) AskPartnerIdentifierAndValidateStep(_ context.Context, _ fsm.Event) (fsm.StateFn, error) {
	return nil, nil
}

// TestPostgresStore_Put_LongBoundMethodStateFnName is a regression test for
// the go-kit incident: oxpulse-bootstrap-bot's Postgres-backed onboarding
// flow never persisted a single session (0 rows in bootstrap_bot_sessions,
// ever) because funcName() (fsm.go) has no length bound, and a bound-method
// StateFn's reflection-derived name (e.g.
// ".../bootstrap.(*KitBot).fsmAskPartnerID-fm", ~86 chars in prod) overflows
// a fixed-width VARCHAR step/flow column. TestFuncName_* in fsm_test.go only
// exercises MemoryStore with short top-level functions and cannot catch
// this — the gap is Postgres's actual column constraint, not funcName()
// itself.
func TestPostgresStore_Put_LongBoundMethodStateFnName(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
	if err != nil {
		t.Fatalf("NewPostgresStore: %v", err)
	}

	controller := &bootstrapOnboardingController{}
	var fn fsm.StateFn = controller.AskPartnerIdentifierAndValidateStep

	// Guard the fixture itself: if this ever stops exceeding 64 chars (e.g.
	// module path shortened), the test would pass for the wrong reason.
	label := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	if len(label) <= 64 {
		t.Fatalf("fixture label %q is only %d chars; widen the receiver/method name to exceed the historical 64-char VARCHAR bound", label, len(label))
	}

	m := fsm.New(store, func(string) fsm.StateFn { return fn }, time.Hour)

	if err := m.Start(ctx, 42001, "onboard"); err != nil {
		t.Fatalf("Start with a long bound-method StateFn: %v (go-kit incident: Postgres 22001 value-too-long — bootstrap_bot_sessions.step must be TEXT, not a fixed-width VARCHAR)", err)
	}

	got, err := store.Get(ctx, 42001)
	if err != nil {
		t.Fatalf("Get after Start: %v", err)
	}
	if got == nil {
		t.Fatal("Get after Start: session missing")
	}
	if got.Step != label {
		t.Fatalf("Step mismatch: want %q, got %q", label, got.Step)
	}
}

// TestPostgresStore_Migrate_WidensPreExistingNarrowColumns proves that a
// bootstrap_bot_sessions table pre-created by a legacy schema (prod's table
// predated this package and had flow/step VARCHAR(32); this package's own
// CREATE TABLE IF NOT EXISTS has since declared VARCHAR(64), also too
// narrow) self-heals on the next migrate() run. CREATE TABLE IF NOT EXISTS
// alone only helps fresh installs — without an unconditional ALTER COLUMN
// ... TYPE, a pre-existing narrow install never widens, silently, forever.
func TestPostgresStore_Migrate_WidensPreExistingNarrowColumns(t *testing.T) {
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprintf("varchar_%d", width), func(t *testing.T) {
			pool := newTestPool(t)
			ctx := context.Background()

			// Simulate a pre-existing legacy install narrower than (or
			// equal to) this package's previously-declared VARCHAR(64).
			_, err := pool.Exec(ctx, fmt.Sprintf(`
				CREATE TABLE bootstrap_bot_sessions (
					chat_id     BIGINT       PRIMARY KEY,
					flow        VARCHAR(%d)  NOT NULL,
					step        VARCHAR(%d)  NOT NULL,
					state_json  JSONB        NOT NULL DEFAULT '{}'::jsonb,
					updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
					expires_at  TIMESTAMPTZ  NOT NULL
				)
			`, width, width))
			if err != nil {
				t.Fatalf("pre-create legacy table: %v", err)
			}

			// NewPostgresStore runs migrate() against the pre-existing table.
			store, err := fsm.NewPostgresStore(ctx, pool, time.Hour)
			if err != nil {
				t.Fatalf("NewPostgresStore: %v", err)
			}

			for _, col := range []string{"flow", "step"} {
				var dataType string
				qErr := pool.QueryRow(ctx, `
					SELECT data_type FROM information_schema.columns
					WHERE table_name = 'bootstrap_bot_sessions' AND column_name = $1
				`, col).Scan(&dataType)
				if qErr != nil {
					t.Fatalf("introspect column %q: %v", col, qErr)
				}
				if dataType != "text" {
					t.Fatalf("column %q: want widened to text, got %q (pre-existing VARCHAR(%d) install never self-healed)", col, dataType, width)
				}
			}

			// Functional proof, not just metadata: the real incident's
			// reproduction (a long bound-method StateFn label) must now fit
			// even though this table started life as a narrow VARCHAR.
			controller := &bootstrapOnboardingController{}
			var fn fsm.StateFn = controller.AskPartnerIdentifierAndValidateStep
			m := fsm.New(store, func(string) fsm.StateFn { return fn }, time.Hour)
			if startErr := m.Start(ctx, int64(50000+width), "onboard"); startErr != nil {
				t.Fatalf("Start after widening a pre-existing VARCHAR(%d) table: %v", width, startErr)
			}
		})
	}
}
