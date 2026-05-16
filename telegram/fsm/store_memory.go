package fsm

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-process Store backed by sync.Map.
// Sessions do not survive process restarts. Use for tests and stateless bots.
type MemoryStore struct {
	m sync.Map // chatID (int64) → *Session
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// Get returns the session for chatID, or nil if absent or expired.
func (s *MemoryStore) Get(_ context.Context, chatID int64) (*Session, error) {
	v, ok := s.m.Load(chatID)
	if !ok {
		return nil, nil
	}
	sess, _ := v.(*Session)
	if sess == nil || time.Now().After(sess.ExpiresAt) {
		return nil, nil
	}
	// Return a shallow copy to prevent callers from mutating stored state.
	cp := *sess
	state := make(map[string]any, len(sess.State))
	for k, v := range sess.State {
		state[k] = v
	}
	cp.State = state
	return &cp, nil
}

// Put stores (or replaces) the session for sess.ChatID.
func (s *MemoryStore) Put(_ context.Context, sess *Session) error {
	// Store a copy.
	cp := *sess
	state := make(map[string]any, len(sess.State))
	for k, v := range sess.State {
		state[k] = v
	}
	cp.State = state
	s.m.Store(sess.ChatID, &cp)
	return nil
}

// Delete removes the session unconditionally.
func (s *MemoryStore) Delete(_ context.Context, chatID int64) error {
	s.m.Delete(chatID)
	return nil
}

// Sweep deletes all sessions where ExpiresAt < now().
func (s *MemoryStore) Sweep(_ context.Context) (int, error) {
	now := time.Now()
	deleted := 0
	s.m.Range(func(key, value any) bool {
		sess, _ := value.(*Session)
		if sess != nil && now.After(sess.ExpiresAt) {
			s.m.Delete(key)
			deleted++
		}
		return true
	})
	return deleted, nil
}
