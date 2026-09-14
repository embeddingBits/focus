-- 0002 adds the session kind: 'focus' default, 'break' for break-mode
-- sessions. Existing rows backfill to 'focus' via the column default.
ALTER TABLE sessions ADD COLUMN kind TEXT NOT NULL DEFAULT 'focus';
