package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	session "github.com/anatolykoptev/go-kit/session"
	redisstore "github.com/anatolykoptev/go-kit/session/redis"
	"github.com/redis/go-redis/v9"
)

// TestPersistError_CountedAndObservable verifies that a Redis failure during
// modify() is no longer silently swallowed: the PersistErrors() counter
// increments so callers can detect the failure. Before the fix, modify()
// discarded the error with `_ = s.persist(...)` and there was no signal at
// all — data was silently lost.
func TestPersistError_CountedAndObservable(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	s := redisstore.New(client, redisstore.Options{
		Prefix:      "test:",
		TTL:         time.Minute,
		MaxMessages: 10,
	})

	// Write a message while Redis is up — should succeed, 0 errors.
	s.AddMessage("k1", session.Message{Role: "user", Content: "hello"})
	if got := s.PersistErrors(); got != 0 {
		t.Fatalf("PersistErrors=%d, want 0 after successful write", got)
	}

	// Close the miniredis server — all subsequent Redis calls will fail.
	mr.Close()

	// SetSummary calls modify → load (connection refused) → error path.
	// Before the fix this was silently swallowed; now PersistErrors
	// increments and the error is logged via slog.Warn.
	s.SetSummary("k1", "a summary")

	if got := s.PersistErrors(); got != 1 {
		t.Fatalf("PersistErrors=%d, want 1 after Redis failure", got)
	}
}
