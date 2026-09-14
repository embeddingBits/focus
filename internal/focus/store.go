// Package focus — persistence port (frozen plan §6, workstream A owns).
//
// SessionRecord is the persistence DTO — what storage reads/writes.
// Store is the interface storage.SQLiteStore implements and the CLI consumes.
package focus

import (
	"context"
	"time"
)

// Session kind values — the shared break-mode contract (workstream A owns
// persistence, workstream B owns the break TUI). A break records a visible
// history row with Kind == KindBreak while StatsToday counts only KindFocus
// rows, so breaks stay out of daily totals.
const (
	KindFocus = "focus"
	KindBreak = "break"
)

// SessionRecord is the persistence DTO — what storage reads/writes.
type SessionRecord struct {
	ID             int64
	Task           string
	Kind           string // KindFocus default, KindBreak for breaks; "" normalizes to KindFocus on Create
	PlannedSeconds int64
	StartedAt      time.Time  // UTC
	EndedAt        *time.Time // UTC, nil while running (Phase 1: only completed rows read back)
	PausedSeconds  int64
	Accomplishment string
	Next           string
	Completed      bool
}

// Store is the persistence port implemented by internal/storage.
type Store interface {
	Migrate(ctx context.Context) error
	Create(ctx context.Context, r SessionRecord) (int64, error)
	Complete(ctx context.Context, id int64, endedAt time.Time, pausedSeconds int64, accomplishment, next string, completed bool) error
	List(ctx context.Context, limit int) ([]SessionRecord, error) // newest first
	StatsToday(ctx context.Context, now time.Time) (total time.Duration, count int, avg time.Duration, err error)
	Close() error
}
