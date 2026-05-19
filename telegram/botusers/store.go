package botusers

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by Store implementations.
var (
	// ErrBotIDRequired is returned when an operation requires a bot_id but
	// none was supplied via the call argument and no default was set via
	// WithBotID.
	ErrBotIDRequired = errors.New("botusers: bot_id required")

	// ErrNotFound is returned by Get and Forget when the user record does
	// not exist for the given (bot_id, tg_id) pair.
	ErrNotFound = errors.New("botusers: user not found")
)

// Store defines the read/write contract for the botusers package.
// Implementations must be safe for concurrent use from multiple goroutines.
//
// The bot_id argument to each method overrides the WithBotID default. If
// both are empty the method returns ErrBotIDRequired.
//
// When WithPrivacy(Off) is active, Upsert* methods silently succeed without
// writing any data.
type Store interface {
	// UpsertFromInitData records or updates a user from a Telegram Mini App
	// init_data payload, where the full TelegramUser struct is available.
	UpsertFromInitData(ctx context.Context, botID string, user TelegramUser, obs Observation) error

	// UpsertFromCommand records or updates a user from a bot command (e.g.
	// /start). Only chatID and lang are reliably available in this path.
	UpsertFromCommand(ctx context.Context, botID string, chatID int64, lang string, obs Observation) error

	// Get returns the user record for (botID, tgID). Returns ErrNotFound
	// when no record exists.
	Get(ctx context.Context, botID string, tgID int64) (*User, error)

	// List returns a page of users matching filter. The second return value
	// is the cursor for the next page; it is zero when no more pages exist.
	List(ctx context.Context, filter Filter) ([]*User, Cursor, error)

	// Aggregate returns summary statistics for the given bot.
	Aggregate(ctx context.Context, botID string) (Aggregates, error)

	// Forget deletes the user record (and event rows when the events table
	// is enabled). Returns ErrNotFound when no record exists.
	Forget(ctx context.Context, botID string, tgID int64) error

	// DeleteInactive removes users whose last_seen_at is older than
	// olderThan relative to now. Returns the count of deleted rows.
	DeleteInactive(ctx context.Context, botID string, olderThan time.Duration) (int64, error)
}
