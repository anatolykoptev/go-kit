package fsm_test

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/telegram/fsm"
)

func TestMemoryStore_GetMissing(t *testing.T) {
	store := fsm.NewMemoryStore()
	sess, err := store.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get missing: unexpected error: %v", err)
	}
	if sess != nil {
		t.Fatalf("Get missing: want nil, got %+v", sess)
	}
}

func TestMemoryStore_PutGet(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	s := &fsm.Session{
		ChatID:    1,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{"foo": "bar"},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Put(ctx, s); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if got == nil {
		t.Fatal("Get after Put: got nil")
	}
	if got.Flow != "onboard" || got.Step != "ask_id" {
		t.Fatalf("Get after Put: wrong fields: %+v", got)
	}
	if got.State["foo"] != "bar" {
		t.Fatalf("Get after Put: wrong state: %+v", got.State)
	}
}

func TestMemoryStore_GetExpired(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	s := &fsm.Session{
		ChatID:    2,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{},
		UpdatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Second), // already expired
	}
	if err := store.Put(ctx, s); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := store.Get(ctx, 2)
	if err != nil {
		t.Fatalf("Get expired: %v", err)
	}
	if got != nil {
		t.Fatalf("Get expired: expected nil for expired session, got %+v", got)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	s := &fsm.Session{
		ChatID:    3,
		Flow:      "onboard",
		Step:      "ask_id",
		State:     map[string]any{},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Put(ctx, s); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, 3); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.Get(ctx, 3)
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if got != nil {
		t.Fatalf("Get after Delete: expected nil, got %+v", got)
	}
}

func TestMemoryStore_Sweep(t *testing.T) {
	store := fsm.NewMemoryStore()
	ctx := context.Background()

	// Add one expired, one alive.
	expired := &fsm.Session{
		ChatID:    10,
		Flow:      "f",
		Step:      "s",
		State:     map[string]any{},
		UpdatedAt: time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(-time.Second),
	}
	alive := &fsm.Session{
		ChatID:    11,
		Flow:      "f",
		Step:      "s",
		State:     map[string]any{},
		UpdatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Put(ctx, expired); err != nil {
		t.Fatalf("Put expired: %v", err)
	}
	if err := store.Put(ctx, alive); err != nil {
		t.Fatalf("Put alive: %v", err)
	}

	deleted, err := store.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("Sweep: want 1 deleted, got %d", deleted)
	}

	// Alive session must still be accessible.
	got, err := store.Get(ctx, 11)
	if err != nil || got == nil {
		t.Fatalf("Sweep: alive session gone: err=%v, got=%v", err, got)
	}

	// Expired session must be gone.
	got, err = store.Get(ctx, 10)
	if err != nil || got != nil {
		t.Fatalf("Sweep: expired session still there: err=%v, got=%v", err, got)
	}
}
