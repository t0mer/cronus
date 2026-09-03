-- Cronus initial schema.

CREATE TABLE IF NOT EXISTS servers (
    id         TEXT PRIMARY KEY,
    address    TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS measurements (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id       TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    ts              TIMESTAMP NOT NULL,
    reachable       INTEGER NOT NULL DEFAULT 0,
    offset_seconds  REAL NOT NULL DEFAULT 0,
    rtt_seconds     REAL NOT NULL DEFAULT 0,
    jitter_seconds  REAL NOT NULL DEFAULT 0,
    stratum         INTEGER NOT NULL DEFAULT 0,
    leap            INTEGER NOT NULL DEFAULT 0,
    resolved_ip     TEXT NOT NULL DEFAULT '',
    reference_id    TEXT NOT NULL DEFAULT '',
    error           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_measurements_server_ts
    ON measurements (server_id, ts);
