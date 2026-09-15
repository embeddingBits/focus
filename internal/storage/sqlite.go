// Package storage implements focus.Store on SQLite via modernc.org/sqlite
// (pure Go, no cgo). Only this package imports the driver.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/focus-cli/focus/internal/focus"
)

// SQLiteStore is a single-writer SQLite implementation of focus.Store.
type SQLiteStore struct {
	db *sql.DB
}

// Open opens the database at dbPath and returns a store. Creating parent
// dirs as needed is the caller's job (see config.Load). The caller must
// call Migrate before use and Close when done.
func Open(dbPath string) (*SQLiteStore, error) {
	dsn := "file:" + dbPath + "?cache=shared"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Single-writer discipline: one *sql.DB per process, one connection.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Migrate applies pending embedded migrations (idempotent).
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	return applyMigrations(ctx, s.db)
}

// Close closes the underlying database.
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close sqlite: %w", err)
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339, raw)
}

func parseNullableTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	t, err := parseTime(raw.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Create inserts a new session row and returns its id. An empty Kind
// normalizes to focus.KindFocus so pre-contract callers stay valid.
func (s *SQLiteStore) Create(ctx context.Context, r focus.SessionRecord) (int64, error) {
	var ended sql.NullString
	if r.EndedAt != nil {
		ended = sql.NullString{String: formatTime(*r.EndedAt), Valid: true}
	}
	completed := 0
	if r.Completed {
		completed = 1
	}
	kind := r.Kind
	if kind == "" {
		kind = focus.KindFocus
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (task, kind, planned_seconds, started_at, ended_at, paused_seconds, accomplishment, next, completed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Task, kind, r.PlannedSeconds, formatTime(r.StartedAt), ended, r.PausedSeconds,
		r.Accomplishment, r.Next, completed,
	)
	if err != nil {
		return 0, fmt.Errorf("insert session: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

// Complete fills in the finished fields of an existing row.
func (s *SQLiteStore) Complete(ctx context.Context, id int64, endedAt time.Time, pausedSeconds int64, accomplishment, next string, completed bool) error {
	done := 0
	if completed {
		done = 1
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET ended_at = ?, paused_seconds = ?, accomplishment = ?, next = ?, completed = ?
		WHERE id = ?`,
		formatTime(endedAt), pausedSeconds, accomplishment, next, done, id,
	)
	if err != nil {
		return fmt.Errorf("complete session %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("complete session %d rows affected: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("complete session %d: no such session", id)
	}
	return nil
}

const sessionColumns = `id, task, kind, planned_seconds, started_at, ended_at, paused_seconds, accomplishment, next, completed`

func scanRecord(row interface {
	Scan(dest ...any) error
}) (focus.SessionRecord, error) {
	var r focus.SessionRecord
	var startedRaw string
	var endedRaw sql.NullString
	var completedInt int
	if err := row.Scan(&r.ID, &r.Task, &r.Kind, &r.PlannedSeconds, &startedRaw, &endedRaw,
		&r.PausedSeconds, &r.Accomplishment, &r.Next, &completedInt); err != nil {
		return focus.SessionRecord{}, err
	}
	started, err := parseTime(startedRaw)
	if err != nil {
		return focus.SessionRecord{}, fmt.Errorf("parse started_at: %w", err)
	}
	r.StartedAt = started
	ended, err := parseNullableTime(endedRaw)
	if err != nil {
		return focus.SessionRecord{}, fmt.Errorf("parse ended_at: %w", err)
	}
	r.EndedAt = ended
	r.Completed = completedInt != 0
	return r, nil
}

// List returns sessions newest-first. A limit <= 0 means no limit.
func (s *SQLiteStore) List(ctx context.Context, limit int) ([]focus.SessionRecord, error) {
	query := `SELECT ` + sessionColumns + ` FROM sessions ORDER BY started_at DESC`
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = s.db.QueryContext(ctx, query+` LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []focus.SessionRecord
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return out, nil
}

// dayBounds returns the local-midnight bounds of the day containing now.
func dayBounds(now time.Time) (start, end time.Time) {
	y, m, d := now.Date()
	start = time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return start, start.Add(24 * time.Hour)
}

// StatsToday returns today's (local-day) total focused time, completed session
// count, and average. Only completed focus sessions count; abandoned rows and
// breaks are excluded (breaks stay visible in history via List). Elapsed per
// session is wall-clock (ended-started) minus paused. Honest numbers, not planned.
func (s *SQLiteStore) StatsToday(ctx context.Context, now time.Time) (total time.Duration, count int, avg time.Duration, err error) {
	dayStart, dayEnd := dayBounds(now)
	rows, err := s.db.QueryContext(ctx, `
		SELECT started_at, ended_at, paused_seconds FROM sessions
		WHERE completed = 1 AND kind = 'focus' AND started_at >= ? AND started_at < ?`,
		formatTime(dayStart), formatTime(dayEnd),
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("stats today: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var startedRaw string
		var endedRaw sql.NullString
		var paused int64
		if err := rows.Scan(&startedRaw, &endedRaw, &paused); err != nil {
			return 0, 0, 0, fmt.Errorf("stats today scan: %w", err)
		}
		if !endedRaw.Valid || endedRaw.String == "" {
			continue
		}
		started, err := parseTime(startedRaw)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("stats today parse started_at: %w", err)
		}
		ended, err := parseTime(endedRaw.String)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("stats today parse ended_at: %w", err)
		}
		d := max(ended.Sub(started)-time.Duration(paused)*time.Second, 0)
		total += d
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, fmt.Errorf("stats today: %w", err)
	}
	if count > 0 {
		avg = total / time.Duration(count)
	}
	return total, count, avg, nil
}

// Compile-time check: SQLiteStore implements focus.Store.
var _ focus.Store = (*SQLiteStore)(nil)
