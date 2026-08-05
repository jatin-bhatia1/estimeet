-- PostgreSQL twin of schema.sql. The tables, constraints and indexes are the
-- same; only the type names differ: BIGINT for the millisecond timestamps,
-- BOOLEAN for the flags SQLite keeps as integers, and BYTEA for the encrypted
-- credentials. Statements are applied one at a time (see splitStatements), so
-- every one of them ends in a semicolon and none contains another.

CREATE TABLE IF NOT EXISTS rooms (
    id               TEXT PRIMARY KEY,
    code             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK (mode IN ('sync', 'async')),
    deck             TEXT NOT NULL,
    current_topic_id TEXT,
    auto_reveal      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       BIGINT NOT NULL,
    closed_at        BIGINT,
    expected_size    INTEGER NOT NULL DEFAULT 0,
    expected_names   TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS participants (
    id           TEXT PRIMARY KEY,
    room_id      TEXT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    is_host      BOOLEAN NOT NULL DEFAULT FALSE,
    is_observer  BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at    BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_participants_room ON participants (room_id);

CREATE TABLE IF NOT EXISTS topics (
    id             TEXT PRIMARY KEY,
    room_id        TEXT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    external_key   TEXT,
    external_url   TEXT,
    position       INTEGER NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('pending', 'voting', 'revealed', 'estimated')),
    final_estimate TEXT,
    created_at     BIGINT NOT NULL,
    revealed_at    BIGINT
);

CREATE INDEX IF NOT EXISTS idx_topics_room_position ON topics (room_id, position);

CREATE UNIQUE INDEX IF NOT EXISTS idx_topics_room_external ON topics (room_id, external_key)
    WHERE external_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS votes (
    topic_id       TEXT NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    participant_id TEXT NOT NULL REFERENCES participants (id) ON DELETE CASCADE,
    value          TEXT NOT NULL,
    created_at     BIGINT NOT NULL,
    PRIMARY KEY (topic_id, participant_id)
);

CREATE INDEX IF NOT EXISTS idx_votes_participant ON votes (participant_id);

CREATE TABLE IF NOT EXISTS source_connections (
    room_id          TEXT PRIMARY KEY REFERENCES rooms (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('jira', 'azure', 'github')),
    auth_type        TEXT NOT NULL CHECK (auth_type IN ('oauth', 'token')),
    base_url         TEXT NOT NULL DEFAULT '',
    cloud_id         TEXT NOT NULL DEFAULT '',
    display_name     TEXT NOT NULL DEFAULT '',
    account          TEXT NOT NULL DEFAULT '',
    access_token     BYTEA NOT NULL,
    refresh_token    BYTEA,
    token_expires_at BIGINT NOT NULL,
    expires_at       BIGINT NOT NULL,
    created_at       BIGINT NOT NULL,
    updated_at       BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_source_connections_expiry ON source_connections (expires_at);

CREATE TABLE IF NOT EXISTS oauth_states (
    state          TEXT PRIMARY KEY,
    room_id        TEXT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    participant_id TEXT NOT NULL,
    code_verifier  TEXT NOT NULL,
    expires_at     BIGINT NOT NULL
);
