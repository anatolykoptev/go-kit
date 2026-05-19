// Package botusers tracks Telegram users that interact with a bot.
//
// # Privacy posture
//
// The package supports three privacy modes, set via WithPrivacy:
//
//   - Off — Upsert is a no-op; no data is written. Use during development
//     or when the bot has no legitimate interest to store user data.
//   - SoftOptIn (default) — data is written for every user that sends a
//     message; users can call Forget to delete their record.
//   - HardOptIn — data is written only after the caller explicitly marks
//     the user as opted-in via a separate product flow. Upsert is a no-op
//     until the user is opted-in.
//
// # Multi-tenancy
//
// Every record is scoped by bot_id. The composite primary key is
// (bot_id, tg_id), so the same Telegram user can have independent records
// across multiple bots sharing one database.
//
// bot_id must be supplied either per-call (botID argument) or as the
// package-wide default via WithBotID. If both are empty, operations return
// ErrBotIDRequired.
//
// # Goroutine safety
//
// Store implementations must be safe for concurrent use from multiple
// goroutines. The package itself never starts goroutines — lifecycle
// management (background sweeper, periodic gauge emission) is the caller's
// responsibility.
//
// # PII handling
//
// The package never logs PII (usernames, first/last names, IPs) at INFO
// or above. Errors include bot_id and tg_id for tracing but never include
// free-text user data.
//
// # Events table
//
// Per-user event history is disabled by default. Enable via
// WithEventsTable(true). When enabled, every Upsert also inserts a row
// into bot_user_events with the observation data. This is append-only;
// Forget deletes both the user row and all event rows.
package botusers
