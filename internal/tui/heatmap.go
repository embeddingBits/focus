package tui

import (
	"strings"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// DayHeat is one local-day bucket of completed focus activity.
type DayHeat struct {
	Date  time.Time // local midnight of the day
	Total time.Duration
	Count int
}

// BuildWeekHeat buckets completed focus sessions into the rolling 7-day
// window ending on now's local day (oldest first). Breaks, abandoned rows,
// and in-progress rows are excluded, consistent with StatsToday. Each
// session's full honest duration (wall-clock minus paused) lands on its
// local start day.
func BuildWeekHeat(records []focus.SessionRecord, now time.Time) []DayHeat {
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	days := make([]DayHeat, 7)
	for i := range days {
		days[i].Date = today.AddDate(0, 0, i-6)
	}

	index := make(map[string]int, 7)
	for i, day := range days {
		index[day.Date.Format("2006-01-02")] = i
	}

	for _, r := range records {
		if !r.Completed || r.Kind == focus.KindBreak || r.EndedAt == nil {
			continue
		}
		dur := r.EndedAt.Sub(r.StartedAt) - time.Duration(r.PausedSeconds)*time.Second
		if dur < 0 {
			dur = 0
		}
		key := r.StartedAt.In(loc).Format("2006-01-02")
		i, ok := index[key]
		if !ok {
			continue
		}
		days[i].Total += dur
		days[i].Count++
	}
	return days
}

// heatBlock maps a day total to one intensity cell.
func heatBlock(total time.Duration) string {
	switch {
	case total <= 0:
		return "·"
	case total < 15*time.Minute:
		return "░"
	case total < time.Hour:
		return "▒"
	case total < 2*time.Hour:
		return "▓"
	default:
		return "█"
	}
}

// RenderHeatmap renders a 7-day activity grid plus per-day lines as plain
// styled text. No TTY needed; used by `focus heatmap`.
func RenderHeatmap(days []DayHeat) string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Heatmap") + "\n")

	if len(days) == 0 {
		b.WriteString(styleDim.Render("No completed sessions this week.") + "\n")
		return b.String()
	}

	var total time.Duration
	var count int
	cells := make([]string, 0, len(days))
	for _, d := range days {
		total += d.Total
		count += d.Count
		cells = append(cells, heatBlock(d.Total))
	}

	if count <= 0 {
		b.WriteString(styleDim.Render("No completed sessions this week.") + "\n")
		return b.String()
	}

	b.WriteString(strings.Join(cells, " ") + "\n")

	for _, d := range days {
		line := d.Date.Format("Mon 01-02") + "  " + heatBlock(d.Total) + "  "
		if d.Count <= 0 {
			line += styleDim.Render("—")
		} else {
			line += styleTime.Render(fmtDur(d.Total)) + " across " + styleTime.Render(formatCount(d.Count))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\nWeek: " + styleTime.Render(fmtDur(total)) +
		" across " + styleTime.Render(formatCount(count)) + "\n")
	return b.String()
}
