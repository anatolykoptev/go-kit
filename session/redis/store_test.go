package redis_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	session "github.com/anatolykoptev/go-kit/session"
	redisstore "github.com/anatolykoptev/go-kit/session/redis"
	"github.com/redis/go-redis/v9"
)

func redisURL() string {
	if u := os.Getenv("REDIS_URL"); u != "" {
		return u
	}
	return "redis://localhost:6379/15"
}

func newTestStore(t *testing.T) *redisstore.Store {
	t.Helper()

	opt, err := redis.ParseURL(redisURL())
	if err != nil {
		t.Skipf("REDIS_URL invalid: %v", err)
	}

	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })

	prefix := fmt.Sprintf("test:%s:", t.Name())
	s := redisstore.New(client, redisstore.Options{
		Prefix:      prefix,
		TTL:         time.Minute,
		MaxMessages: 10,
	})

	t.Cleanup(func() {
		// best-effort key cleanup: Delete known keys touched in test
	})

	return s
}

func pingRedis(t *testing.T) {
	t.Helper()
	opt, err := redis.ParseURL(redisURL())
	if err != nil {
		t.Skipf("skip: REDIS_URL invalid: %v", err)
	}
	client := redis.NewClient(opt)
	defer func() { _ = client.Close() }()
	if _, err = client.Ping(t.Context()).Result(); err != nil {
		t.Skipf("skip: Redis unavailable at %s: %v", redisURL(), err)
	}
}

func TestGetOrCreate(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	sess := s.GetOrCreate("user1")
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.Key != "user1" {
		t.Errorf("key=%q want user1", sess.Key)
	}
}

func TestAddMessageAndGetHistory(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "hist1"
	s.AddMessage(key, session.Message{Role: "user", Content: "hello"})
	s.AddMessage(key, session.Message{Role: "assistant", Content: "hi"})

	msgs := s.GetHistory(key)
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("msgs[0].Content=%q want hello", msgs[0].Content)
	}
}

func TestMessageCount(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "count1"
	if got := s.MessageCount(key); got != 0 {
		t.Errorf("empty session count=%d want 0", got)
	}

	s.AddMessage(key, session.Message{Role: "user", Content: "a"})
	s.AddMessage(key, session.Message{Role: "user", Content: "b"})

	if got := s.MessageCount(key); got != 2 {
		t.Errorf("count=%d want 2", got)
	}
}

func TestMaxMessages(t *testing.T) {
	pingRedis(t)
	opt, _ := redis.ParseURL(redisURL())
	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })

	s := redisstore.New(client, redisstore.Options{
		Prefix:      fmt.Sprintf("test:%s:", t.Name()),
		TTL:         time.Minute,
		MaxMessages: 3,
	})

	key := "maxmsgs"
	for i := range 5 {
		s.AddMessage(key, session.Message{Role: "user", Content: fmt.Sprintf("msg%d", i)})
	}

	if got := s.MessageCount(key); got != 3 {
		t.Errorf("count=%d want 3 (capped)", got)
	}
}

func TestGetSummaryAndSetSummary(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "sum1"
	if got := s.GetSummary(key); got != "" {
		t.Errorf("initial summary=%q want empty", got)
	}

	s.SetSummary(key, "a nice summary")
	if got := s.GetSummary(key); got != "a nice summary" {
		t.Errorf("summary=%q want 'a nice summary'", got)
	}
}

func TestGetFactsAndAddFacts(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "facts1"
	if got := s.GetFacts(key); got != nil {
		t.Errorf("initial facts should be nil, got %v", got)
	}

	facts := []session.Fact{
		{Content: "sky is blue", ExtractedAt: time.Now()},
		{Content: "water is wet", ExtractedAt: time.Now()},
	}
	s.AddFacts(key, facts)

	got := s.GetFacts(key)
	if len(got) != 2 {
		t.Fatalf("got %d facts, want 2", len(got))
	}
	if got[0].Content != "sky is blue" {
		t.Errorf("fact[0]=%q want 'sky is blue'", got[0].Content)
	}
}

func TestCompactMessages(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "compact1"
	for i := range 5 {
		s.AddMessage(key, session.Message{Role: "user", Content: fmt.Sprintf("m%d", i)})
	}

	removed := s.CompactMessages(key, 2)
	if len(removed) != 3 {
		t.Errorf("removed=%d want 3", len(removed))
	}
	if s.MessageCount(key) != 2 {
		t.Errorf("remaining=%d want 2", s.MessageCount(key))
	}
}

func TestTruncateHistory(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "trunc1"
	for i := range 6 {
		s.AddMessage(key, session.Message{Role: "user", Content: fmt.Sprintf("x%d", i)})
	}

	s.TruncateHistory(key, 3)
	if got := s.MessageCount(key); got != 3 {
		t.Errorf("count=%d want 3", got)
	}
}

func TestClear(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "clear1"
	s.AddMessage(key, session.Message{Role: "user", Content: "yo"})
	s.SetSummary(key, "summ")
	s.AddFacts(key, []session.Fact{{Content: "fact"}})

	s.Clear(key)

	if s.MessageCount(key) != 0 {
		t.Error("messages not cleared")
	}
	if s.GetSummary(key) != "" {
		t.Error("summary not cleared")
	}
	if s.GetFacts(key) != nil {
		t.Error("facts not cleared")
	}
}

func TestDeleteAndSave(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	key := "del1"
	s.AddMessage(key, session.Message{Role: "user", Content: "bye"})

	if err := s.Save(key); err != nil {
		t.Errorf("Save returned error: %v", err)
	}
	if err := s.Delete(key); err != nil {
		t.Errorf("Delete returned error: %v", err)
	}

	if s.MessageCount(key) != 0 {
		t.Error("session still has messages after Delete")
	}
}

func TestListStale(t *testing.T) {
	pingRedis(t)
	s := newTestStore(t)

	// All freshly written sessions should NOT be stale.
	s.AddMessage("fresh1", session.Message{Role: "user", Content: "hi"})

	stale := s.ListStale(time.Hour)
	for _, k := range stale {
		if k == "fresh1" {
			t.Error("fresh1 incorrectly listed as stale")
		}
	}
}

func TestImplementsStoreInterface(t *testing.T) {
	// Compile-time assertion only; no Redis needed.
	var _ session.Store = (*redisstore.Store)(nil)
}
