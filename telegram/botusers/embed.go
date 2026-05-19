package botusers

import _ "embed"

//go:embed schema.sql
var schemaSQL string

// SchemaSQL returns the DDL for the bot_users (and optionally bot_user_events)
// tables. The SQL is idempotent: all statements use CREATE TABLE IF NOT EXISTS
// and CREATE INDEX IF NOT EXISTS, so it can be applied multiple times safely.
//
// The pg sub-package's Store.Apply method runs this SQL via pgxpool. Callers
// that manage migrations externally can embed this string directly.
func SchemaSQL() string { return schemaSQL }
