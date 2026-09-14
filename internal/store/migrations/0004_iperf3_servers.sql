CREATE TABLE iperf3_servers (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    host             TEXT NOT NULL,
    port             INTEGER NOT NULL,
    options          TEXT NOT NULL DEFAULT '',
    supports_reverse INTEGER NOT NULL DEFAULT 0,
    supports_udp     INTEGER NOT NULL DEFAULT 0,
    gbs              TEXT NOT NULL DEFAULT '',
    continent        TEXT NOT NULL DEFAULT '',
    country          TEXT NOT NULL DEFAULT '',
    site             TEXT NOT NULL DEFAULT '',
    provider         TEXT NOT NULL DEFAULT '',
    UNIQUE(host, port)
);

CREATE TABLE iperf3_meta (
    k TEXT PRIMARY KEY,
    v TEXT NOT NULL
);
