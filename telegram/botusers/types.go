package botusers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Privacy controls whether and how user data is stored.
type Privacy int

const (
	// Off disables all data storage. Upsert calls are no-ops.
	Off Privacy = iota
	// SoftOptIn is the default. Data is written for every user;
	// users can remove their record via Forget.
	SoftOptIn
	// Note: a future HardOptIn mode (opted_in BOOLEAN column + MarkOptedIn/IsOptedIn API)
	// is planned for a later release.
)

// TelegramUser carries the fields available from Telegram InitData or a
// /start message. Fields may be zero-valued when the source does not
// provide them.
type TelegramUser struct {
	// TgID is the Telegram user ID. Required.
	TgID int64
	// Username is the @username, without the leading @. May be empty.
	Username string
	// FirstName is the user's first name. May be empty.
	FirstName string
	// LastName is the user's last name. May be empty.
	LastName string
	// Lang is the IETF language tag reported by Telegram (e.g. "ru", "en").
	Lang string
	// IsPremium indicates the user has a Telegram Premium subscription.
	IsPremium bool
	// IsBot is true when the user is a bot account.
	IsBot bool
}

// Observation carries context about a single user interaction. The Store
// implementation decides which fields to persist based on configuration.
type Observation struct {
	// IP is the client IP address. Only stored when WithStoreIP is true;
	// otherwise ignored.
	IP string
	// Platform is the client platform (e.g. "ios", "android", "web", "desktop").
	Platform string
	// Country is the ISO 3166-1 alpha-2 country code, if known.
	Country string
	// LangDetected is the language detected from message content, if any.
	LangDetected string
	// Source identifies the write path. Use the package constants:
	// SourceMiniAppInit or SourceBotCommand.
	Source string
	// At is the observation timestamp. Defaults to time.Now() when zero.
	At time.Time
}

// Source constants for Observation.Source.
const (
	// SourceMiniAppInit indicates the observation came from a Mini App
	// init_data payload.
	SourceMiniAppInit = "miniapp_init"
	// SourceBotCommand indicates the observation came from a bot command
	// such as /start.
	SourceBotCommand = "bot_command"
)

// User is the stored representation of a Telegram user within a bot scope.
type User struct {
	// BotID identifies the bot this record belongs to.
	BotID string
	// TgID is the Telegram user ID.
	TgID int64
	// Username is the @username. May be empty.
	Username string
	// FirstName is the user's first name.
	FirstName string
	// LastName is the user's last name.
	LastName string
	// Lang is the Telegram-reported IETF language tag.
	Lang string
	// IsPremium indicates a Telegram Premium subscription.
	IsPremium bool
	// IsBot is true for bot accounts.
	IsBot bool
	// Country is the ISO 3166-1 alpha-2 country code. May be empty;
	// populated externally (e.g. via GeoIP on the IP from an Observation).
	Country string
	// Platform is the client platform. May be empty.
	Platform string
	// FirstSeenAt is the timestamp of the first observation. Immutable
	// after creation — re-upserts must not change this value.
	FirstSeenAt time.Time
	// LastSeenAt is the timestamp of the most recent observation.
	LastSeenAt time.Time
	// TotalObservations counts how many times Upsert has been called for
	// this user. Monotonically increasing.
	TotalObservations int64
}

// Filter selects a subset of users for List.
type Filter struct {
	// BotID scopes the query. Required — List returns ErrBotIDRequired
	// if both BotID and the store's WithBotID default are empty.
	BotID string
	// Lang filters by Telegram-reported language tag when non-empty.
	Lang string
	// Countries filters by ISO country codes (OR semantics).
	Countries []string
	// Platforms filters by platform strings (OR semantics).
	Platforms []string
	// Premium filters by premium status. Nil means no filter.
	Premium *bool
	// ActiveWithin restricts results to users whose last_seen_at is within
	// this duration of now(). Nil means no filter.
	ActiveWithin *time.Duration
	// Query is a free-text search over username, first_name, last_name.
	// Implementation-specific; may use ILIKE or full-text search.
	Query string
	// Cursor is the pagination continuation token from the previous List call.
	// Nil means start from the beginning.
	Cursor *Cursor
	// Limit is the maximum number of users to return. Defaults to 50 when <= 0.
	Limit int
}

// Cursor is an opaque pagination token. Callers must not inspect or
// construct Cursor values directly — use only the value returned by List.
type Cursor struct {
	// encoded holds the base64-encoded JSON payload.
	encoded string
}

// cursorPayload is the internal structure encoded into a Cursor.
type cursorPayload struct {
	LastSeenAt time.Time `json:"ls"`
	TgID       int64     `json:"id"`
}

// EncodeCursor creates a Cursor from a last_seen_at / tg_id pair. Used by
// Store implementers.
func EncodeCursor(lastSeenAt time.Time, tgID int64) Cursor {
	p := cursorPayload{LastSeenAt: lastSeenAt, TgID: tgID}
	raw, _ := json.Marshal(p) //nolint:errcheck // struct is always marshallable
	return Cursor{encoded: base64.RawURLEncoding.EncodeToString(raw)}
}

// DecodeCursor extracts the last_seen_at / tg_id pair from c. Used by
// Store implementers. Returns a wrapped ErrCursor on any decode failure
// so callers can detect tampered cursors via errors.Is(err, ErrCursor).
func DecodeCursor(c Cursor) (lastSeenAt time.Time, tgID int64, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(c.encoded)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("%w: %s", ErrCursor, err.Error())
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return time.Time{}, 0, fmt.Errorf("%w: %s", ErrCursor, err.Error())
	}
	return p.LastSeenAt, p.TgID, nil
}

// String returns the encoded cursor string. Safe to embed in URLs.
func (c Cursor) String() string { return c.encoded }

// CursorFromString rebuilds a Cursor from a previously serialised string.
func CursorFromString(s string) Cursor { return Cursor{encoded: s} }

// IsZero reports whether the cursor is empty (i.e. "start from beginning").
func (c Cursor) IsZero() bool { return c.encoded == "" }

// CountryCount holds a country code and the count of users from that country.
type CountryCount struct {
	// Code is the ISO 3166-1 alpha-2 country code (e.g. "US", "DE").
	Code string
	// Count is the number of stored users from this country.
	Count int64
}

// Aggregates summarises the user base for a single bot.
type Aggregates struct {
	// Total is the total number of stored users.
	Total int64
	// Active1D is the count of users seen in the last 24 hours.
	Active1D int64
	// Active7D is the count of users seen in the last 7 days.
	Active7D int64
	// Active30D is the count of users seen in the last 30 days.
	Active30D int64
	// PremiumCount is the count of premium users.
	PremiumCount int64
	// TopCountries is an ordered list of CountryCount values, most frequent first.
	TopCountries []CountryCount
}
