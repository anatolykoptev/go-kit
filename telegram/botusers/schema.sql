-- bot_users stores one record per (bot_id, tg_id) pair.
-- Multi-tenancy: multiple bots can share one database via the bot_id column.
-- Privacy: rows are written only when the configured Privacy mode allows it.
-- All timestamps are stored with timezone (TIMESTAMPTZ).

CREATE TABLE IF NOT EXISTS bot_users (
    bot_id              TEXT        NOT NULL,
    tg_id               BIGINT      NOT NULL,
    username            TEXT        NOT NULL DEFAULT '',
    first_name          TEXT        NOT NULL DEFAULT '',
    last_name           TEXT        NOT NULL DEFAULT '',
    lang                TEXT        NOT NULL DEFAULT '',
    is_premium          BOOLEAN     NOT NULL DEFAULT FALSE,
    is_bot              BOOLEAN     NOT NULL DEFAULT FALSE,
    country             TEXT        NOT NULL DEFAULT '',
    platform            TEXT        NOT NULL DEFAULT '',
    -- first_seen_at is set on INSERT and never updated (immutable).
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- total_observations counts every Upsert call; incremented by 1 each time.
    total_observations  BIGINT      NOT NULL DEFAULT 1,
    -- client_ip is only populated when WithStoreIP is enabled.
    client_ip           TEXT        NOT NULL DEFAULT '',
    CONSTRAINT bot_users_pkey PRIMARY KEY (bot_id, tg_id)
);

-- Index for active-user window queries (Aggregate + DeleteInactive).
CREATE INDEX IF NOT EXISTS idx_bot_users_last_seen
    ON bot_users (bot_id, last_seen_at DESC);

-- Index for List pagination (keyset: last_seen_at DESC, tg_id ASC).
CREATE INDEX IF NOT EXISTS idx_bot_users_cursor
    ON bot_users (bot_id, last_seen_at DESC, tg_id ASC);

-- Index for country aggregation.
CREATE INDEX IF NOT EXISTS idx_bot_users_country
    ON bot_users (bot_id, country)
    WHERE country != '';

-- Optional events table. Created by Apply only when WithEventsTable is true.
-- DDL is included here for completeness; pg.Store.Apply applies it conditionally.
CREATE TABLE IF NOT EXISTS bot_user_events (
    id          BIGSERIAL   PRIMARY KEY,
    bot_id      TEXT        NOT NULL,
    tg_id       BIGINT      NOT NULL,
    source      TEXT        NOT NULL DEFAULT '',
    platform    TEXT        NOT NULL DEFAULT '',
    country     TEXT        NOT NULL DEFAULT '',
    client_ip   TEXT        NOT NULL DEFAULT '',
    lang        TEXT        NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bot_user_events_user
    ON bot_user_events (bot_id, tg_id, occurred_at DESC);
