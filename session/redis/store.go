// Package redis provides a Redis-backed session store.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	session "github.com/anatolykoptev/go-kit/session"
	"github.com/redis/go-redis/v9"
)

const (
	defaultPrefix       = "session:"
	scanPageSize        = 100
	contentTruncSuffix  = "\n... [truncated]"
)

// Store is a Redis-backed implementation of session.Store.
// Each session is stored as a JSON blob under "<prefix><key>".
// All writes refresh the Redis TTL. Save is a no-op.
type Store struct {
	client     *redis.Client
	prefix     string
	ttl        time.Duration
	maxMsgs    int
	maxContent int
	maxFacts   int
	mu         sync.Mutex // guards read-modify-write cycles
}

// Options configures the Redis store.
type Options struct {
	Prefix         string        // Redis key prefix (default: "session:")
	TTL            time.Duration // applied on every write; 0 = no expiry
	MaxMessages    int           // max history length on AddMessage; 0 = unlimited
	MaxContentSize int           // truncate message content beyond this byte length; 0 = unlimited
	MaxFacts       int           // rotate oldest facts when exceeded; 0 = unlimited
}

// New creates a Store backed by the given client.
func New(client *redis.Client, opts Options) *Store {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = defaultPrefix
	}
	return &Store{
		client:     client,
		prefix:     prefix,
		ttl:        opts.TTL,
		maxMsgs:    opts.MaxMessages,
		maxContent: opts.MaxContentSize,
		maxFacts:   opts.MaxFacts,
	}
}

func (s *Store) redisKey(key string) string { return s.prefix + key }

func (s *Store) load(ctx context.Context, key string) (*session.Session, error) {
	data, err := s.client.Get(ctx, s.redisKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return session.NewSession(key), nil
	}
	if err != nil {
		return nil, err
	}
	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) persist(ctx context.Context, sess *session.Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, s.redisKey(sess.Key), data, s.ttl).Err()
}

// modify executes a locked read-modify-write against Redis.
func (s *Store) modify(key string, fn func(*session.Session)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx := context.Background()
	sess, err := s.load(ctx, key)
	if err != nil {
		return
	}
	fn(sess)
	_ = s.persist(ctx, sess) //nolint:errcheck // best-effort persistence
}

func (s *Store) truncateContent(msg *session.Message) {
	if s.maxContent > 0 && len(msg.Content) > s.maxContent {
		msg.Content = msg.Content[:s.maxContent] + contentTruncSuffix
	}
}

func (s *Store) enforceFacts(sess *session.Session) {
	if s.maxFacts > 0 && len(sess.Facts) > s.maxFacts {
		sess.Facts = sess.Facts[len(sess.Facts)-s.maxFacts:]
	}
}

// GetOrCreate returns an existing session or creates a new one.
func (s *Store) GetOrCreate(key string) *session.Session {
	sess, err := s.load(context.Background(), key)
	if err != nil {
		return session.NewSession(key)
	}
	return sess
}

// AddMessage appends a message, enforcing MaxMessages and MaxContentSize.
func (s *Store) AddMessage(key string, msg session.Message) {
	s.truncateContent(&msg)
	s.modify(key, func(sess *session.Session) {
		sess.AddMessage(msg)
		if s.maxMsgs > 0 && len(sess.Messages) > s.maxMsgs {
			sess.Messages = sess.Messages[len(sess.Messages)-s.maxMsgs:]
		}
	})
}

// UpdateLastMessage replaces the content of the most recent message.
func (s *Store) UpdateLastMessage(key string, content string) {
	if s.maxContent > 0 && len(content) > s.maxContent {
		content = content[:s.maxContent] + contentTruncSuffix
	}
	s.modify(key, func(sess *session.Session) {
		if len(sess.Messages) == 0 {
			return
		}
		sess.Messages[len(sess.Messages)-1].Content = content
		sess.Updated = time.Now()
	})
}

// GetHistory returns an ordered copy of the session's messages.
func (s *Store) GetHistory(key string) []session.Message {
	sess, err := s.load(context.Background(), key)
	if err != nil || len(sess.Messages) == 0 {
		return nil
	}
	out := make([]session.Message, len(sess.Messages))
	copy(out, sess.Messages)
	return out
}

// GetSummary returns the compaction summary for a session.
func (s *Store) GetSummary(key string) string {
	sess, err := s.load(context.Background(), key)
	if err != nil {
		return ""
	}
	return sess.Summary
}

// SetSummary stores a compaction summary.
func (s *Store) SetSummary(key, summary string) {
	s.modify(key, func(sess *session.Session) {
		sess.Summary = summary
		sess.Updated = time.Now()
	})
}

// GetFacts returns a copy of extracted facts.
func (s *Store) GetFacts(key string) []session.Fact {
	sess, err := s.load(context.Background(), key)
	if err != nil {
		return nil
	}
	return sess.GetFacts()
}

// AddFacts appends new facts to a session, enforcing MaxFacts.
func (s *Store) AddFacts(key string, facts []session.Fact) {
	s.modify(key, func(sess *session.Session) {
		sess.AddFacts(facts)
		s.enforceFacts(sess)
	})
}

// MessageCount returns the number of messages in a session.
func (s *Store) MessageCount(key string) int {
	sess, err := s.load(context.Background(), key)
	if err != nil {
		return 0
	}
	return sess.MessageCount()
}

// CompactMessages extracts the oldest messages keeping keepLast, and persists.
func (s *Store) CompactMessages(key string, keepLast int) []session.Message {
	var removed []session.Message
	s.modify(key, func(sess *session.Session) { removed = sess.CompactMessages(keepLast) })
	return removed
}

// TruncateHistory removes oldest messages, keeping keepLast.
func (s *Store) TruncateHistory(key string, keepLast int) {
	s.modify(key, func(sess *session.Session) { sess.TruncateHistory(keepLast) })
}

// Clear resets a session's messages, summary, and facts.
func (s *Store) Clear(key string) {
	s.modify(key, func(sess *session.Session) { sess.Clear() })
}

// Delete removes a session from Redis.
func (s *Store) Delete(key string) error {
	return s.client.Del(context.Background(), s.redisKey(key)).Err()
}

// Save is a no-op; all operations auto-persist to Redis.
func (s *Store) Save(_ string) error { return nil }

// ListStale scans keys with the store's prefix, returning those whose
// Updated timestamp is older than maxAge.
func (s *Store) ListStale(maxAge time.Duration) []string {
	ctx := context.Background()
	cutoff := time.Now().Add(-maxAge)
	pattern := s.prefix + "*"

	var stale []string
	var cursor uint64

	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, scanPageSize).Result()
		if err != nil {
			break
		}
		for _, rk := range keys {
			key := rk[len(s.prefix):]
			if sess, err := s.load(ctx, key); err == nil && sess.Updated.Before(cutoff) {
				stale = append(stale, key)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return stale
}
