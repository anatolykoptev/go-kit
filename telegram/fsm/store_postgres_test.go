package fsm_test

import (
	"context"
	"os"
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
