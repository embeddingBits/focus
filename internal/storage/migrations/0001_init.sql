CREATE TABLE IF NOT EXISTS sessions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    task             TEXT NOT NULL,
    planned_seconds  INTEGER NOT NULL,
    started_at       TEXT NOT NULL,   -- RFC3339 UTC
    ended_at         TEXT,            -- RFC3339 UTC, NULL until finished
    paused_seconds   INTEGER NOT NULL DEFAULT 0,
    accomplishment   TEXT NOT NULL DEFAULT '',
    next             TEXT NOT NULL DEFAULT '',
    completed        INTEGER NOT NULL DEFAULT 0  -- 0/1
);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
