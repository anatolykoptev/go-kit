package fsm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is a Store backed by PostgreSQL via pgxpool.
//
// Schema: bootstrap_bot_sessions — preserves the production table used by
// oxpulse-admin/internal/bootstrap/sessions.go (unmerged feat/bot-session-engine).
// PR 5 migration is drop-in: no ALTER TABLE required, column names match.
//
// Direct port of postgresSessionStore from that branch; CreatedAt removed
// per spec §4 (Session struct has no CreatedAt field).
type PostgresStore struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

// NewPostgresStore creates a PostgresStore and runs an idempotent schema
// migration (CREATE TABLE IF NOT EXISTS).
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool, ttl time.Duration) (*PostgresStore, error) {
	s := &PostgresStore{pool: pool, ttl: ttl}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	// flow/step hold funcName()'s reflection-derived StateFn label (fsm.go),
	// which has no length bound — a bound method's name is the full
	// package-import-path + receiver type + method + "-fm" suffix, routinely
	// >64 chars. A fixed-width VARCHAR is the wrong constraint for that; use
	// TEXT (unbounded, same on-disk representation, no perf cost in
	// Postgres).
	//
	// CREATE TABLE IF NOT EXISTS only helps fresh installs. A pre-existing
	// table (e.g. one that predates this package, or was created before this
	// column was widened) never gets touched by it, so the ALTER COLUMN
	// below runs unconditionally on every migrate() call to self-heal any
	// narrower install. VARCHAR(n)->TEXT is metadata-only in Postgres (no
	// table rewrite) and a no-op when the column is already TEXT — safe to
	// run on every startup.
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bootstrap_bot_sessions (
			chat_id     BIGINT       PRIMARY KEY,
			flow        TEXT         NOT NULL,
			step        TEXT         NOT NULL,
			state_json  JSONB        NOT NULL DEFAULT '{}'::jsonb,
			updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
			expires_at  TIMESTAMPTZ  NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_bootstrap_bot_sessions_expires
			ON bootstrap_bot_sessions(expires_at);

		ALTER TABLE bootstrap_bot_sessions ALTER COLUMN flow TYPE TEXT;
		ALTER TABLE bootstrap_bot_sessions ALTER COLUMN step TYPE TEXT;
	`)
	if err != nil {
		return fmt.Errorf("fsm: postgres migration: %w", err)
	}
	return nil
}

// Get returns the session for chatID if it exists and has not expired.
// Returns (nil, nil) for absent or expired sessions.
func (s *PostgresStore) Get(ctx context.Context, chatID int64) (*Session, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT chat_id, flow, step, state_json, updated_at, expires_at
		FROM bootstrap_bot_sessions
		WHERE chat_id = $1 AND expires_at > now()
	`, chatID)

	var sess Session
	var stateJSON []byte
	err := row.Scan(
		&sess.ChatID, &sess.Flow, &sess.Step, &stateJSON,
		&sess.UpdatedAt, &sess.ExpiresAt,
	)
	if err != nil {
		// No row — not an error per Store contract.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		// All other errors (context.DeadlineExceeded, conn failures, etc.) propagate.
		return nil, fmt.Errorf("fsm: get session: %w", err)
	}

	if err := json.Unmarshal(stateJSON, &sess.State); err != nil {
		return nil, fmt.Errorf("fsm: unmarshal session state: %w", err)
	}
	return &sess, nil
}

// Put creates or replaces the session. updated_at and expires_at are reset
// to now() and now()+ttl on every call.
func (s *PostgresStore) Put(ctx context.Context, sess *Session) error {
	stateJSON, err := json.Marshal(sess.State)
	if err != nil {
		return fmt.Errorf("fsm: marshal session state: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(s.ttl)

	_, err = s.pool.Exec(ctx, `
		INSERT INTO bootstrap_bot_sessions
			(chat_id, flow, step, state_json, updated_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (chat_id) DO UPDATE SET
			flow       = EXCLUDED.flow,
			step       = EXCLUDED.step,
			state_json = EXCLUDED.state_json,
			updated_at = EXCLUDED.updated_at,
			expires_at = EXCLUDED.expires_at
	`, sess.ChatID, sess.Flow, sess.Step, stateJSON, now, expiresAt)
	if err != nil {
		return fmt.Errorf("fsm: upsert session: %w", err)
	}
	return nil
}

// Delete removes the session unconditionally.
func (s *PostgresStore) Delete(ctx context.Context, chatID int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM bootstrap_bot_sessions WHERE chat_id = $1`, chatID)
	if err != nil {
		return fmt.Errorf("fsm: delete session: %w", err)
	}
	return nil
}

// Sweep deletes all sessions where expires_at < now(). Returns deleted count.
func (s *PostgresStore) Sweep(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM bootstrap_bot_sessions WHERE expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("fsm: sweep sessions: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
