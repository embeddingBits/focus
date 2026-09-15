package tui

import (
	"strconv"
	"strings"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// RenderStats renders today's totals plus per-session lines as plain styled
// text (frozen plan §10, exact signature). Totals come from the store's
// StatsToday; records back the per-session lines. No TTY needed.
func RenderStats(total time.Duration, count int, avg time.Duration, records []focus.SessionRecord) string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Stats") + "\n")

	if count <= 0 {
		b.WriteString(styleDim.Render("No completed sessions today.") + "\n")
		return b.String()
	}

	b.WriteString("Today: " + styleTime.Render(fmtDur(total)) +
		" across " + styleTime.Render(formatCount(count)) +
		" (avg " + styleTime.Render(fmtDur(avg)) + ")" + "\n\n")

	for _, r := range records {
		b.WriteString(formatSessionLine(r) + "\n")
	}
	return b.String()
}

func formatCount(n int) string {
	if n == 1 {
		return "1 session"
	}
	return strconv.Itoa(n) + " sessions"
}
