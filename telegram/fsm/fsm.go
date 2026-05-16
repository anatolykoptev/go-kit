// Package fsm provides a conversation state machine for Telegram bots.
//
// # Overview
//
// A Machine holds active multi-turn conversations keyed by chat_id. Each
// conversation is a chain of StateFn values: every incoming message is
// dispatched to the current StateFn, which returns the next StateFn
// (or nil to end the conversation).
//
// # Stores
//
// Two Store implementations are provided:
//   - NewMemoryStore — sync.Map; suitable for tests and stateless bots.
//   - NewPostgresStore — pgxpool-backed; preserves sessions across restarts.
//
// # StateFn contract
//
// A StateFn receives a context and an Event and returns (next StateFn, error).
//
//   - Return the same function to stay in the current step (re-prompt user).
//   - Return a different function to advance.
//   - Return nil to end the conversation (Machine deletes the session).
//   - Return a non-nil error to signal failure; the Machine propagates it to
//     the caller without deleting the session.
//
// # Feed semantics
//
// Feed returns (handled=false, nil) when no active session exists for the
// chatID (missing or expired). Callers should treat handled=false as "not in
// a multi-turn flow" and route the update through their default handler.
//
// Note: promise-chain mutex is non-re-entrant; do not call Feed from a
// running StateFn.
package fsm

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Event is the unit of input dispatched to a StateFn.
type Event struct {
	ChatID int64
	UserID int64
	Text   string           // message text or callback data
	Update *tgbotapi.Update // escape hatch for any field not covered above
}

// StateFn is a state in a conversation flow.
// It processes an Event and returns the next state (or nil to end).
type StateFn func(ctx context.Context, e Event) (StateFn, error)

// Session holds the persistent state for one active conversation.
type Session struct {
	ChatID    int64
	Flow      string
	Step      string         // human-readable label for observability
	State     map[string]any // free-form, JSON-marshalable
	UpdatedAt time.Time
	ExpiresAt time.Time
}

// Store is the persistence abstraction for conversation sessions.
type Store interface {
	// Get returns the session for chatID, or (nil, nil) if absent or expired.
	Get(ctx context.Context, chatID int64) (*Session, error)
	// Put creates or replaces (upserts) the session.
	Put(ctx context.Context, s *Session) error
	// Delete removes the session unconditionally.
	Delete(ctx context.Context, chatID int64) error
	// Sweep deletes all sessions where ExpiresAt < now(). Returns count deleted.
	Sweep(ctx context.Context) (deleted int, err error)
}

// Machine dispatches incoming Telegram events to the correct StateFn.
type Machine struct {
	store   Store
	initial func(flow string) StateFn
	ttl     time.Duration
	cfg     machineConfig
	// fnCache maps chatID (int64) → StateFn for the current step.
	// Populated by Start and updated by each Feed transition.
	// Cleared by Cancel and terminal-state transitions.
	fnCache sync.Map
}

// New creates a Machine.
//
//   - store: where sessions are persisted.
//   - initial: factory that returns the first StateFn for a named flow.
//   - ttl: inactivity timeout; sessions older than ttl are swept.
//   - opts: optional configuration (WithCancelCmds, WithOnCancel, …).
func New(store Store, initial func(flow string) StateFn, ttl time.Duration, opts ...Option) *Machine {
	var cfg machineConfig
	for _, o := range opts {
		o(&cfg)
	}
	return &Machine{store: store, initial: initial, ttl: ttl, cfg: cfg}
}

// Start creates a new session for chatID in the given flow. If a session
// already exists it is overwritten (restart semantics).
func (m *Machine) Start(ctx context.Context, chatID int64, flow string) error {
	fn := m.initial(flow)
	if fn == nil {
		return fmt.Errorf("fsm: unknown flow %q", flow)
	}
	m.storeFn(chatID, fn)
	step := funcName(fn)
	now := time.Now()
	sess := &Session{
		ChatID:    chatID,
		Flow:      flow,
		Step:      step,
		State:     make(map[string]any),
		UpdatedAt: now,
		ExpiresAt: now.Add(m.ttl),
	}
	if err := m.store.Put(ctx, sess); err != nil {
		return fmt.Errorf("fsm.Start: %w", err)
	}
	return nil
}

// Feed dispatches e to the current StateFn for e.ChatID.
//
// Returns (false, nil) when no active session exists (absent or expired).
// Returns (true, err) when a StateFn ran — err is nil on success, non-nil on
// StateFn error or recovered panic.
//
// When a StateFn returns nil (end of flow) the session is deleted.
// When a StateFn returns itself the session persists at the same Step.
// When a StateFn returns an error the session is NOT deleted (caller may retry
// or cancel).
func (m *Machine) Feed(ctx context.Context, e Event) (handled bool, err error) {
	sess, err := m.store.Get(ctx, e.ChatID)
	if err != nil {
		return false, fmt.Errorf("fsm.Feed get: %w", err)
	}
	if sess == nil {
		// No active session — let caller decide what to do.
		return false, nil
	}

	// Check if the incoming text is a cancel command. If so, end the session
	// without dispatching to the StateFn and invoke the OnCancel hook.
	if len(m.cfg.cancelCmds) > 0 {
		if _, ok := m.cfg.cancelCmds[e.Text]; ok {
			m.clearFn(e.ChatID)
			if delErr := m.store.Delete(ctx, e.ChatID); delErr != nil {
				return true, fmt.Errorf("fsm.Feed cancel delete: %w", delErr)
			}
			if m.cfg.onCancel != nil {
				m.cfg.onCancel(ctx, e.ChatID)
			}
			return true, nil
		}
	}

	// Look up the current StateFn from the in-process cache (populated by
	// Start and updated on each Feed transition). On process restart with
	// PostgresStore the cache is empty; fall back to initial(flow) which
	// re-starts the flow from step 1 — the consumer must call Start again
	// for mid-flow recovery (out of scope for this package).
	fn := m.loadFn(e.ChatID)
	if fn == nil {
		fn = m.initial(sess.Flow)
		if fn == nil {
			return false, fmt.Errorf("fsm.Feed: unknown flow %q (no fn cache, restart?)", sess.Flow)
		}
	}

	next, fnErr := m.callFn(ctx, fn, e)
	if fnErr != nil {
		return true, fnErr
	}

	if next == nil {
		// Terminal state — delete session and clear cache.
		m.clearFn(e.ChatID)
		if delErr := m.store.Delete(ctx, e.ChatID); delErr != nil {
			return true, fmt.Errorf("fsm.Feed delete: %w", delErr)
		}
		return true, nil
	}

	// Advance (or stay) — update Step + ExpiresAt in the store.
	m.storeFn(e.ChatID, next)
	now := time.Now()
	sess.Step = funcName(next)
	sess.UpdatedAt = now
	sess.ExpiresAt = now.Add(m.ttl)
	if putErr := m.store.Put(ctx, sess); putErr != nil {
		return true, fmt.Errorf("fsm.Feed put: %w", putErr)
	}
	return true, nil
}

// Cancel deletes the session for chatID. Safe to call with no active session.
func (m *Machine) Cancel(ctx context.Context, chatID int64) error {
	m.clearFn(chatID)
	if err := m.store.Delete(ctx, chatID); err != nil {
		return fmt.Errorf("fsm.Cancel: %w", err)
	}
	return nil
}

// StartSweeper launches a goroutine that calls Store.Sweep every interval.
// It exits when ctx is cancelled.
func (m *Machine) StartSweeper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := m.store.Sweep(ctx)
				if err != nil {
					slog.Warn("fsm sweep error", slog.String("err", err.Error()))
				} else if n > 0 {
					slog.Info("fsm sweep", slog.Int("deleted", n))
				}
			}
		}
	}()
}

// ---- function cache (in-process) ----
//
// We use a sync.Map to map chatID → StateFn. This avoids any global lock and
// is safe for concurrent Feed calls on different chatIDs.

// callFn invokes fn with panic recovery. A panic is converted to an error;
// the Machine state is not mutated (no fn stored, no session written).
func (m *Machine) callFn(ctx context.Context, fn StateFn, e Event) (next StateFn, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("fsm: panic in StateFn: %v\n%s", r, debug.Stack())
		}
	}()
	return fn(ctx, e)
}

func funcName(fn StateFn) string {
	// We use the function pointer identity via fmt.Sprintf as a best-effort
	// step label. Consumers should name their StateFn vars descriptively.
	if fn == nil {
		return ""
	}
	return fmt.Sprintf("%p", fn)
}

func (m *Machine) loadFn(chatID int64) StateFn {
	v, ok := m.fnCache.Load(chatID)
	if !ok {
		return nil
	}
	fn, _ := v.(StateFn)
	return fn
}

func (m *Machine) storeFn(chatID int64, fn StateFn) {
	m.fnCache.Store(chatID, fn)
}

func (m *Machine) clearFn(chatID int64) {
	m.fnCache.Delete(chatID)
}
