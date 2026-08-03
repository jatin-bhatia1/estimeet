CREATE TABLE IF NOT EXISTS rooms (
    id               TEXT PRIMARY KEY,
    code             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK (mode IN ('sync', 'async')),
    deck             TEXT NOT NULL,
    current_topic_id TEXT,
    auto_reveal      INTEGER NOT NULL DEFAULT 1,
    created_at       INTEGER NOT NULL,
    closed_at        INTEGER
);

CREATE TABLE IF NOT EXISTS participants (
    id           TEXT PRIMARY KEY,
    room_id      TEXT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    is_host      INTEGER NOT NULL DEFAULT 0,
    is_observer  INTEGER NOT NULL DEFAULT 0,
    joined_at    INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
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
    created_at     INTEGER NOT NULL,
    revealed_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_topics_room_position ON topics (room_id, position);
CREATE UNIQUE INDEX IF NOT EXISTS idx_topics_room_external ON topics (room_id, external_key)
    WHERE external_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS votes (
    topic_id       TEXT NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    participant_id TEXT NOT NULL REFERENCES participants (id) ON DELETE CASCADE,
    value          TEXT NOT NULL,
    created_at     INTEGER NOT NULL,
    PRIMARY KEY (topic_id, participant_id)
);

CREATE INDEX IF NOT EXISTS idx_votes_participant ON votes (participant_id);

-- One Jira Cloud connection per room. Tokens are encrypted at rest (AES-256-GCM).
CREATE TABLE IF NOT EXISTS jira_connections (
    room_id       TEXT PRIMARY KEY REFERENCES rooms (id) ON DELETE CASCADE,
    cloud_id      TEXT NOT NULL,
    site_url      TEXT NOT NULL,
    site_name     TEXT NOT NULL DEFAULT '',
    access_token  BLOB NOT NULL,
    refresh_token BLOB,
    expires_at    INTEGER NOT NULL,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);

-- Short-lived CSRF state for the OAuth 2.0 authorization-code flow.
CREATE TABLE IF NOT EXISTS oauth_states (
    state          TEXT PRIMARY KEY,
    room_id        TEXT NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    participant_id TEXT NOT NULL,
    code_verifier  TEXT NOT NULL,
    expires_at     INTEGER NOT NULL
);
