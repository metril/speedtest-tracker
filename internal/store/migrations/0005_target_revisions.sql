-- target_revisions keeps a full-snapshot change history per target so
-- edits can be reviewed and reverted, and deleted targets can be
-- restored. No foreign key to targets: rows must survive the target row
-- being deleted.
CREATE TABLE target_revisions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id  INTEGER NOT NULL,
    version    INTEGER NOT NULL,
    action     TEXT NOT NULL CHECK(action IN ('create','update','delete','revert','restore')),
    snapshot   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    UNIQUE(target_id, version)
);

CREATE INDEX idx_target_revisions_target_version ON target_revisions(target_id, version DESC);
