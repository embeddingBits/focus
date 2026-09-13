// Package tui — TEMPORARY STUB owned by workstream A.
//
// Workstream B (Bubble Tea UI) owns this package. These stubs expose the
// frozen §10 API so A builds and tests green before B lands. B must delete
// this file (and its siblings) at integration and provide the real
// implementation with identical signatures.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// TimerRequest describes a timer run.
type TimerRequest struct {
	Task    string
	Planned time.Duration
	Elapsed func() time.Duration // optional hook; default wall-clock from time.Now
}

// TimerResult is the outcome of a timer run.
type TimerResult struct {
	Completed      bool
	Accomplishment string
	Next           string
	ElapsedTotal   time.Duration
	PausedTotal    time.Duration
}

// RunTimer is a STUB: it reports an immediately completed session without
// drawing any UI. Replaced by the Bubble Tea full-screen timer in
// workstream B.
func RunTimer(req TimerRequest) (TimerResult, error) {
	return TimerResult{
		Completed:    true,
		ElapsedTotal: req.Planned,
	}, nil
}

// RenderHistory is a STUB plain-text renderer. Replaced by the lipgloss
// version in workstream B (same signature).
func RenderHistory(records []focus.SessionRecord, limit int) string {
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	if len(records) == 0 {
		return "No sessions yet.\n"
	}
	var b strings.Builder
	for _, r := range records {
		mark := "✓"
		if !r.Completed {
			mark = "✗ abandoned"
		}
		actual := ""
		if r.EndedAt != nil {
			d := r.EndedAt.Sub(r.StartedAt) - time.Duration(r.PausedSeconds)*time.Second
			if d < 0 {
				d = 0
			}
			actual = fmt.Sprintf(" (%s)", d.Round(time.Second))
		}
		fmt.Fprintf(&b, "%s %s — %s [%s]%s\n",
			r.StartedAt.Format("2006-01-02 15:04"), mark, r.Task,
			(time.Duration(r.PlannedSeconds) * time.Second).Round(time.Second), actual)
	}
	return b.String()
}

// RenderStats is a STUB plain-text renderer. Replaced by the lipgloss version
// in workstream B (same signature).
func RenderStats(total time.Duration, count int, avg time.Duration, records []focus.SessionRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Today: %s across %d sessions (avg %s)\n",
		total.Round(time.Second), count, avg.Round(time.Second))
	for _, r := range records {
		fmt.Fprintf(&b, "  • %s — %s\n", r.Task,
			r.StartedAt.Format("15:04"))
	}
	return b.String()
}
