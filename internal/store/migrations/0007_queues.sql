-- Queues replace the fixed wan/lan lane pair with a user-managed list.
-- Targets in the same queue run one at a time; different queues run in
-- parallel (this is exactly what the old lane concept did, generalised).

CREATE TABLE queues (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL UNIQUE,
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Seed wan/lan first (in that order) so a fresh database always gets
-- wan=1, lan=2: store code treats the lowest-id queue as the default for
-- new targets and for resolving legacy "lane" snapshots. Any other
-- distinct lane already present in targets is seeded too, so no existing
-- target is left pointing at a lane with no matching queue.
INSERT INTO queues(name) VALUES ('wan'), ('lan');
INSERT INTO queues(name)
  SELECT DISTINCT lane FROM targets WHERE lane NOT IN ('wan','lan') ORDER BY lane;

-- The lane column is kept (old target_revisions snapshots reference it by
-- name) but is no longer written by the application; queue_id is now the
-- live source of truth.
ALTER TABLE targets ADD COLUMN queue_id INTEGER NOT NULL DEFAULT 1 REFERENCES queues(id);

UPDATE targets SET queue_id = (SELECT id FROM queues WHERE queues.name = targets.lane);
