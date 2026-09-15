CREATE TABLE sessions (
    id         TEXT PRIMARY KEY, -- sha256 hex of the session cookie token
    subject    TEXT NOT NULL,
    email      TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    groups     TEXT NOT NULL DEFAULT '[]',
    is_admin   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
