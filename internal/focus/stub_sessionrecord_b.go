// Package focus — WORKSTREAM B STUB.
//
// This file is a temporary local stub copied verbatim from the frozen plan
// (data/focus-plan-s1/report.md §6) so workstream B can build and test the
// TUI independently before workstream A lands the canonical engine.
//
// DELETE AT INTEGRATION: workstream A owns internal/focus (store.go etc.).
// When A's SessionRecord lands, delete this file; the TUI code already
// references focus.SessionRecord with the exact frozen shape.
package focus

import "time"

// SessionRecord is the persistence DTO — what storage reads/writes.
type SessionRecord struct {
	ID             int64
	Task           string
	PlannedSeconds int64
	StartedAt      time.Time  // UTC
	EndedAt        *time.Time // UTC, nil while running (Phase 1: only completed rows read back)
	PausedSeconds  int64
	Accomplishment string
	Next           string
	Completed      bool
}
