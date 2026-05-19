// Package pg provides a PostgreSQL-backed implementation of botusers.Store
// using pgxpool.
//
// Usage:
//
//	pool, err := pgxpool.New(ctx, dsn)
//	store, err := pg.New(ctx, pool, botusers.WithBotID("my-bot"))
//
// New applies the schema (idempotent) on first call. Apply can be called
// again at any time to upgrade tables (statements use IF NOT EXISTS).
package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anatolykoptev/go-kit/telegram/botusers"
)

// Store is a botusers.Store backed by PostgreSQL.
// It is safe for concurrent use from multiple goroutines.
type Store struct {
	pool *pgxpool.Pool
	cfg  botusers.Config
}

// New creates a Store, applies the schema (idempotent), and returns the store.
// opts follows the same pattern as the parent package's functional options.
func New(ctx context.Context, pool *pgxpool.Pool, opts ...botusers.Option) (*Store, error) {
	cfg := botusers.Config{}
	// Apply defaults the same way the parent package does.
	cfg.Privacy = botusers.SoftOptIn
	cfg.SweepInterval = 24 * time.Hour
	cfg.Clock = time.Now
	for _, o := range opts {
		o(&cfg)
	}

	cfg.WarnCrossDomainOptions("store")

	s := &Store{pool: pool, cfg: cfg}
	if err := s.Apply(ctx, pool); err != nil {
		return nil, err
	}
	return s, nil
}

// Apply runs the embedded schema SQL against the given pool. It is idempotent:
// all DDL uses CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT EXISTS.
// Callers that manage migrations externally can skip Apply and run
// botusers.SchemaSQL() themselves.
func (s *Store) Apply(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, botusers.SchemaSQL())
	if err != nil {
		return fmt.Errorf("pg: apply schema: %w", err)
	}
	return nil
}

// resolveBot returns the effective bot_id or ErrBotIDRequired.
func (s *Store) resolveBot(botID string) (string, error) {
	if botID != "" {
		return botID, nil
	}
	if s.cfg.BotID != "" {
		return s.cfg.BotID, nil
	}
	return "", botusers.ErrBotIDRequired
}

// UpsertFromInitData implements botusers.Store.
// When privacy is Off, the call is a no-op.
func (s *Store) UpsertFromInitData(ctx context.Context, botID string, user botusers.TelegramUser, obs botusers.Observation) error {
	if s.cfg.Privacy == botusers.Off {
		return nil
	}
	bid, err := s.resolveBot(botID)
	if err != nil {
		return err
	}
	at := obs.At
	if at.IsZero() {
		at = s.cfg.Clock()
	}

	ip := ""
	if s.cfg.StoreIP {
		ip = obs.IP
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("pg: begin upsert tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// M2: when StoreIP is disabled, unconditionally clear any prior stored IP
	// rather than preserving it via CASE. This ensures disabling the option
	// on subsequent upserts actively purges previously stored values.
	clientIPClause := "client_ip = CASE WHEN EXCLUDED.client_ip != '' THEN EXCLUDED.client_ip ELSE bot_users.client_ip END,"
	if !s.cfg.StoreIP {
		clientIPClause = "client_ip = '',"
	}
	upsertSQL := `
		INSERT INTO bot_users (
			bot_id, tg_id, username, first_name, last_name, lang,
			is_premium, is_bot, country, platform, client_ip,
			first_seen_at, last_seen_at, total_observations, custom_attrs
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $12, 1, '{}'::jsonb
		)
		ON CONFLICT (bot_id, tg_id) DO UPDATE SET
			username           = CASE WHEN EXCLUDED.username != '' THEN EXCLUDED.username ELSE bot_users.username END,
			first_name         = CASE WHEN EXCLUDED.first_name != '' THEN EXCLUDED.first_name ELSE bot_users.first_name END,
			last_name          = CASE WHEN EXCLUDED.last_name != '' THEN EXCLUDED.last_name ELSE bot_users.last_name END,
			lang               = EXCLUDED.lang,
			is_premium         = EXCLUDED.is_premium,
			is_bot             = EXCLUDED.is_bot,
			country            = CASE WHEN EXCLUDED.country != '' THEN EXCLUDED.country ELSE bot_users.country END,
			platform           = CASE WHEN EXCLUDED.platform != '' THEN EXCLUDED.platform ELSE bot_users.platform END,
			` + clientIPClause + `
			last_seen_at       = EXCLUDED.last_seen_at,
			total_observations = bot_users.total_observations + 1
			-- first_seen_at is intentionally NOT updated (immutable after INSERT)
	`
	_, err = tx.Exec(ctx, upsertSQL,
		bid, user.TgID, user.Username, user.FirstName, user.LastName, user.Lang,
		user.IsPremium, user.IsBot, obs.Country, obs.Platform, ip,
		at,
	)
	if err != nil {
		return fmt.Errorf("pg: upsert from init data: %w", err)
	}

	if s.cfg.UseEventsTable {
		if err = s.insertEventTx(ctx, tx, bid, user.TgID, obs, ip, at); err != nil {
			return err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit upsert tx: %w", err)
	}
	return nil
}

// UpsertFromCommand implements botusers.Store.
func (s *Store) UpsertFromCommand(ctx context.Context, botID string, chatID int64, lang string, obs botusers.Observation) error {
	user := botusers.TelegramUser{TgID: chatID, Lang: lang}
	return s.UpsertFromInitData(ctx, botID, user, obs)
}

// insertEventTx writes a row to bot_user_events within an existing transaction.
// Called only when UseEventsTable is true.
func (s *Store) insertEventTx(ctx context.Context, tx pgx.Tx, botID string, tgID int64, obs botusers.Observation, ip string, at time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bot_user_events (bot_id, tg_id, source, platform, country, client_ip, lang, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, botID, tgID, obs.Source, obs.Platform, obs.Country, ip, obs.LangDetected, at)
	if err != nil {
		return fmt.Errorf("pg: insert event: %w", err)
	}
	return nil
}

// Get implements botusers.Store.
func (s *Store) Get(ctx context.Context, botID string, tgID int64) (*botusers.User, error) {
	bid, err := s.resolveBot(botID)
	if err != nil {
		return nil, err
	}

	row := s.pool.QueryRow(ctx, `
		SELECT bot_id, tg_id, username, first_name, last_name, lang,
		       is_premium, is_bot, country, platform,
		       first_seen_at, last_seen_at, total_observations, custom_attrs
		FROM bot_users
		WHERE bot_id = $1 AND tg_id = $2
	`, bid, tgID)

	var u botusers.User
	var customAttrsJSON []byte
	err = row.Scan(
		&u.BotID, &u.TgID, &u.Username, &u.FirstName, &u.LastName, &u.Lang,
		&u.IsPremium, &u.IsBot, &u.Country, &u.Platform,
		&u.FirstSeenAt, &u.LastSeenAt, &u.TotalObservations, &customAttrsJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, botusers.ErrNotFound
		}
		return nil, fmt.Errorf("pg: get user: %w", err)
	}

	u.CustomAttrs = make(map[string]any)
	if len(customAttrsJSON) > 0 && string(customAttrsJSON) != "null" {
		if err := unmarshalJSON(customAttrsJSON, &u.CustomAttrs); err != nil {
			return nil, fmt.Errorf("pg: unmarshal custom_attrs: %w", err)
		}
	}
	return &u, nil
}

// List implements botusers.Store with keyset pagination.
// Ordering: last_seen_at DESC, tg_id ASC.
func (s *Store) List(ctx context.Context, filter botusers.Filter) ([]*botusers.User, botusers.Cursor, error) {
	bid := filter.BotID
	if bid == "" {
		bid = s.cfg.BotID
	}
	if bid == "" {
		return nil, botusers.Cursor{}, botusers.ErrBotIDRequired
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	// Fetch one extra to detect next page.
	fetchLimit := limit + 1

	// Build parameterised query with keyset cursor.
	var args []any
	args = append(args, bid)
	q := `
		SELECT bot_id, tg_id, username, first_name, last_name, lang,
		       is_premium, is_bot, country, platform,
		       first_seen_at, last_seen_at, total_observations, custom_attrs
		FROM bot_users
		WHERE bot_id = $1`

	if filter.Cursor != nil && !filter.Cursor.IsZero() {
		curTs, curID, err := botusers.DecodeCursor(*filter.Cursor)
		if err != nil {
			return nil, botusers.Cursor{}, fmt.Errorf("pg: decode cursor: %w", err)
		}
		args = append(args, curTs, curID)
		q += fmt.Sprintf(` AND (last_seen_at < $%d OR (last_seen_at = $%d AND tg_id > $%d))`,
			len(args)-1, len(args)-1, len(args))
	}

	q += fmt.Sprintf(` ORDER BY last_seen_at DESC, tg_id ASC LIMIT $%d`, len(args)+1)
	args = append(args, fetchLimit)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, botusers.Cursor{}, fmt.Errorf("pg: list users: %w", err)
	}
	defer rows.Close()

	var users []*botusers.User
	for rows.Next() {
		var u botusers.User
		var customAttrsJSON []byte
		if err := rows.Scan(
			&u.BotID, &u.TgID, &u.Username, &u.FirstName, &u.LastName, &u.Lang,
			&u.IsPremium, &u.IsBot, &u.Country, &u.Platform,
			&u.FirstSeenAt, &u.LastSeenAt, &u.TotalObservations, &customAttrsJSON,
		); err != nil {
			return nil, botusers.Cursor{}, fmt.Errorf("pg: scan user: %w", err)
		}
		u.CustomAttrs = make(map[string]any)
		if len(customAttrsJSON) > 0 {
			_ = unmarshalJSON(customAttrsJSON, &u.CustomAttrs) // best-effort
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, botusers.Cursor{}, fmt.Errorf("pg: list rows: %w", err)
	}

	var nextCursor botusers.Cursor
	if len(users) > limit {
		users = users[:limit]
		last := users[len(users)-1]
		nextCursor = botusers.EncodeCursor(last.LastSeenAt, last.TgID)
	}

	return users, nextCursor, nil
}

// Aggregate implements botusers.Store.
func (s *Store) Aggregate(ctx context.Context, botID string) (botusers.Aggregates, error) {
	bid, err := s.resolveBot(botID)
	if err != nil {
		return botusers.Aggregates{}, err
	}

	row := s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE last_seen_at >= now() - INTERVAL '1 day'),
			COUNT(*) FILTER (WHERE last_seen_at >= now() - INTERVAL '7 days'),
			COUNT(*) FILTER (WHERE last_seen_at >= now() - INTERVAL '30 days'),
			COUNT(*) FILTER (WHERE is_premium)
		FROM bot_users
		WHERE bot_id = $1
	`, bid)

	var agg botusers.Aggregates
	if err := row.Scan(&agg.Total, &agg.Active1D, &agg.Active7D, &agg.Active30D, &agg.PremiumCount); err != nil {
		return botusers.Aggregates{}, fmt.Errorf("pg: aggregate: %w", err)
	}

	// Top countries.
	rows, err := s.pool.Query(ctx, `
		SELECT country, COUNT(*) AS cnt
		FROM bot_users
		WHERE bot_id = $1 AND country != ''
		GROUP BY country
		ORDER BY cnt DESC
		LIMIT 10
	`, bid)
	if err != nil {
		return botusers.Aggregates{}, fmt.Errorf("pg: aggregate countries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var cnt int64
		if err := rows.Scan(&code, &cnt); err != nil {
			return botusers.Aggregates{}, fmt.Errorf("pg: scan country: %w", err)
		}
		agg.TopCountries = append(agg.TopCountries, botusers.CountryCount{Code: code, Count: cnt})
	}
	if err := rows.Err(); err != nil {
		return botusers.Aggregates{}, fmt.Errorf("pg: country rows: %w", err)
	}

	return agg, nil
}

// Forget implements botusers.Store. Returns ErrNotFound when no row matches.
// MED1: delete user row first so ErrNotFound fires before touching event rows.
// Both deletes are wrapped in a transaction for atomicity.
func (s *Store) Forget(ctx context.Context, botID string, tgID int64) error {
	bid, err := s.resolveBot(botID)
	if err != nil {
		return err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("pg: begin forget tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Delete user row first; if no row exists, return ErrNotFound without
	// touching the events table (avoids ghost-event orphan confusion).
	var tag interface{ RowsAffected() int64 }
	tag, err = tx.Exec(ctx,
		`DELETE FROM bot_users WHERE bot_id = $1 AND tg_id = $2`, bid, tgID)
	if err != nil {
		return fmt.Errorf("pg: forget user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return botusers.ErrNotFound
	}

	if s.cfg.UseEventsTable {
		_, err = tx.Exec(ctx,
			`DELETE FROM bot_user_events WHERE bot_id = $1 AND tg_id = $2`, bid, tgID)
		if err != nil {
			return fmt.Errorf("pg: forget events: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("pg: commit forget tx: %w", err)
	}
	return nil
}

// DeleteInactive implements botusers.Store.
func (s *Store) DeleteInactive(ctx context.Context, botID string, olderThan time.Duration) (int64, error) {
	bid, err := s.resolveBot(botID)
	if err != nil {
		return 0, err
	}
	cutoff := s.cfg.Clock().Add(-olderThan)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM bot_users WHERE bot_id = $1 AND last_seen_at < $2`, bid, cutoff)
	if err != nil {
		return 0, fmt.Errorf("pg: delete inactive: %w", err)
	}
	return tag.RowsAffected(), nil
}

// unmarshalJSON is a thin wrapper that avoids importing encoding/json at the
// call sites (keeps code DRY). The JSONB bytes from pgx are standard JSON.
func unmarshalJSON(data []byte, v any) error {
	// Using pgx's built-in JSON decoding via standard library.
	// We import encoding/json indirectly; declare import at top of file.
	return json.Unmarshal(data, v)
}

// Ensure *Store satisfies the Store interface at compile time.
var _ botusers.Store = (*Store)(nil)
