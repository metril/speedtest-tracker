CREATE TABLE targets (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL,
  engine     TEXT    NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  lane       TEXT    NOT NULL DEFAULT 'wan',
  options    TEXT    NOT NULL DEFAULT '{}',
  thresholds TEXT    NOT NULL DEFAULT '{}',
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE schedules (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT    NOT NULL UNIQUE,
  cron       TEXT    NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  timezone   TEXT    NOT NULL DEFAULT 'UTC',
  created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE schedule_targets (
  schedule_id INTEGER NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
  target_id   INTEGER NOT NULL REFERENCES targets(id)   ON DELETE CASCADE,
  position    INTEGER NOT NULL,
  PRIMARY KEY (schedule_id, position)
);
CREATE INDEX idx_schedule_targets_target ON schedule_targets(target_id);

CREATE TABLE runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  schedule_id INTEGER REFERENCES schedules(id) ON DELETE SET NULL,
  trigger     TEXT NOT NULL CHECK (trigger IN ('cron','manual','reexec','api')),
  status      TEXT NOT NULL CHECK (status IN ('queued','running','done','failed','canceled','skipped')),
  started_at  TEXT,
  finished_at TEXT,
  error       TEXT
);
CREATE INDEX idx_runs_started_at ON runs(started_at DESC);

CREATE TABLE results (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id           INTEGER REFERENCES runs(id)    ON DELETE CASCADE,
  target_id        INTEGER REFERENCES targets(id) ON DELETE SET NULL,
  target_name      TEXT    NOT NULL,
  engine           TEXT    NOT NULL,
  options_snapshot TEXT    NOT NULL DEFAULT '{}',
  status           TEXT    NOT NULL CHECK (status IN ('ok','failed','degraded')),
  error            TEXT,
  started_at       TEXT    NOT NULL,
  duration_ms      INTEGER,
  download_bps     REAL,
  upload_bps       REAL,
  ping_ms          REAL,
  jitter_ms        REAL,
  packet_loss_pct  REAL,
  bytes_down       INTEGER,
  bytes_up         INTEGER,
  server_id        TEXT,
  server_name      TEXT,
  server_host      TEXT,
  isp              TEXT,
  external_ip      TEXT,
  result_url       TEXT,
  raw              TEXT
);
CREATE INDEX idx_results_target_started ON results(target_id, started_at DESC);
CREATE INDEX idx_results_started        ON results(started_at DESC);
CREATE INDEX idx_results_status_started ON results(status, started_at DESC);

CREATE TABLE tags (
  id   INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE
);

CREATE TABLE result_tags (
  result_id INTEGER NOT NULL REFERENCES results(id) ON DELETE CASCADE,
  tag_id    INTEGER NOT NULL REFERENCES tags(id)    ON DELETE CASCADE,
  PRIMARY KEY (result_id, tag_id)
);
CREATE INDEX idx_result_tags_tag ON result_tags(tag_id);

CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE notification_state (
  target_id     INTEGER NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  metric        TEXT    NOT NULL,
  last_fired_at TEXT,
  firing        INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (target_id, metric)
);
