package tui

import (
	"strings"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// RenderHistory renders past sessions newest-first as plain styled text
// (frozen plan §10, exact signature). No TTY needed. Used by `focus history`.
func RenderHistory(records []focus.SessionRecord, limit int) string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("History") + "\n")

	rows := records
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}

	if len(rows) == 0 {
		b.WriteString(styleDim.Render("No sessions yet. Start one with: focus start \"<task>\"") + "\n")
		return b.String()
	}

	for _, r := range rows {
		b.WriteString(formatSessionLine(r) + "\n")
		if strings.TrimSpace(r.Accomplishment) != "" || strings.TrimSpace(r.Next) != "" {
			b.WriteString(formatSessionDetail(r) + "\n")
		}
	}
	return b.String()
}

// actualDuration is honest wall-clock minus paused time (never planned).
func actualDuration(r focus.SessionRecord) (time.Duration, bool) {
	if r.EndedAt == nil {
		return 0, false
	}
	d := r.EndedAt.Sub(r.StartedAt) - time.Duration(r.PausedSeconds)*time.Second
	if d < 0 {
		d = 0
	}
	return d, true
}

// formatSessionLine is the one-line summary shared by history and stats.
func formatSessionLine(r focus.SessionRecord) string {
	date := r.StartedAt.Format("2006-01-02 15:04")
	planned := time.Duration(r.PlannedSeconds) * time.Second

	length := styleDim.Render("in progress")
	if actual, ok := actualDuration(r); ok {
		length = fmtDur(actual)
	}

	mark := styleBad.Render("abandoned")
	if r.Kind == focus.KindBreak {
		// Break rows are rest, not abandoned work: distinct mark, and no
		// accomplishment/next lines (breaks carry none). "" (pre-migration
		// rows) falls through to the focus rendering below.
		mark = styleLabel.Render("break")
	} else if r.Completed {
		mark = styleOK.Render("completed")
	}

	return date + "  " +
		styleTitle.Render(r.Task) + "  " +
		styleDim.Render(fmtDur(planned)+" → "+length) + "  " +
		mark
}

// formatSessionDetail shows persisted accomplishment/next prompts, if any.
func formatSessionDetail(r focus.SessionRecord) string {
	parts := []string{}
	if strings.TrimSpace(r.Accomplishment) != "" {
		parts = append(parts, "did: "+strings.TrimSpace(r.Accomplishment))
	}
	if strings.TrimSpace(r.Next) != "" {
		parts = append(parts, "next: "+strings.TrimSpace(r.Next))
	}
	return "    " + styleDim.Render(strings.Join(parts, " · "))
}
