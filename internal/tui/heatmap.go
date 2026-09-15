package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// weekdayNames orders columns Monday-first, matching the productivity grid.
var weekdayNames = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// ProductivityGrid holds per-weekday per-hour focused totals over a trailing
// window of days. Cells[mon0][hour] accumulates honest focused time
// (wall-clock minus paused), with sessions spanning hour boundaries split
// proportionally across the hours they cover.
type ProductivityGrid struct {
	WindowDays int
	MinHour    int // inclusive, hours outside hold no activity
	MaxHour    int // inclusive
	Cells      [7][24]time.Duration
	Total      time.Duration
	Count      int
	PeakWD     int // 0 = Monday
	PeakHour   int
	Peak       time.Duration
	MaxCell    time.Duration
}

// mon0 maps a Weekday to a Monday-first column index.
func mon0(wd time.Weekday) int {
	return (int(wd) + 6) % 7
}

// BuildProductivityGrid buckets completed focus sessions from the trailing
// windowDays local days (including today) into weekday × hour cells.
// Breaks, abandoned rows, and in-progress rows are excluded, consistent with
// StatsToday. A windowDays <= 0 means no window limit.
func BuildProductivityGrid(records []focus.SessionRecord, now time.Time, windowDays int) ProductivityGrid {
	var g ProductivityGrid
	g.WindowDays = windowDays
	g.MinHour = -1

	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	winStart := today
	if windowDays > 0 {
		winStart = today.AddDate(0, 0, -(windowDays - 1))
	}

	for _, r := range records {
		if !r.Completed || r.Kind == focus.KindBreak || r.EndedAt == nil {
			continue
		}
		if windowDays > 0 && r.StartedAt.In(loc).Before(winStart) {
			continue
		}
		g.Count++
		splitSession(&g, r, loc)
	}

	for wd := 0; wd < 7; wd++ {
		for h := 0; h < 24; h++ {
			v := g.Cells[wd][h]
			if v <= 0 {
				continue
			}
			g.Total += v
			if v > g.MaxCell {
				g.MaxCell = v
				g.PeakWD = wd
				g.PeakHour = h
				g.Peak = v
			}
			if g.MinHour < 0 || h < g.MinHour {
				g.MinHour = h
			}
			if h > g.MaxHour {
				g.MaxHour = h
			}
		}
	}
	return g
}

// splitSession distributes one session's honest duration across the local
// hour slots it overlaps, proportionally to wall-clock overlap. Pause time
// scales every overlapped hour evenly (paused intervals aren't tracked
// per-hour, so even spreading is the honest approximation).
func splitSession(g *ProductivityGrid, r focus.SessionRecord, loc *time.Location) {
	start, end := r.StartedAt, *r.EndedAt
	wall := end.Sub(start)
	if wall <= 0 {
		return
	}
	honest := wall - time.Duration(r.PausedSeconds)*time.Second
	if honest < 0 {
		honest = 0
	}
	if honest == 0 {
		return
	}
	scale := float64(honest) / float64(wall)

	sLocal := start.In(loc)
	slot := time.Date(sLocal.Year(), sLocal.Month(), sLocal.Day(), sLocal.Hour(), 0, 0, 0, loc)
	eLocal := end.In(loc)
	for slot.Before(eLocal) {
		next := slot.Add(time.Hour)
		ovStart := start
		if slot.After(ovStart) {
			ovStart = slot
		}
		ovEnd := end
		if next.Before(ovEnd) {
			ovEnd = next
		}
		if ov := ovEnd.Sub(ovStart); ov > 0 {
			g.Cells[mon0(slot.Weekday())][slot.Hour()] += time.Duration(math.Round(float64(ov) * scale))
		}
		slot = next
	}
}

// hourCell renders one grid cell, 3 runes wide: more blocks mean more focus
// relative to the hottest cell. Trace amounts that round to zero show as ░.
func hourCell(v, max time.Duration) string {
	if v <= 0 || max <= 0 {
		return "   "
	}
	n := int(math.Round(3 * float64(v) / float64(max)))
	if n <= 0 {
		return "░  "
	}
	if n > 3 {
		n = 3
	}
	return strings.Repeat("█", n) + strings.Repeat(" ", 3-n)
}

// RenderProductivityGrid renders the weekday × hour grid as plain styled
// text. No TTY needed; used by `focus heatmap`.
func RenderProductivityGrid(g ProductivityGrid) string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Heatmap") + "\n")
	b.WriteString(styleDim.Render("When are you most productive? (last "+formatDays(g.WindowDays)+")") + "\n\n")

	if g.MaxCell <= 0 {
		b.WriteString(styleDim.Render("No completed sessions in the last "+formatDays(g.WindowDays)+".") + "\n")
		return b.String()
	}

	b.WriteString("       " + strings.Join(weekdayNames, " ") + "\n\n")

	for h := g.MinHour; h <= g.MaxHour; h++ {
		row := strings.Builder{}
		row.WriteString(styleLabel.Render(fmt.Sprintf("%02d:00", h)) + "  ")
		for wd := 0; wd < 7; wd++ {
			if wd > 0 {
				row.WriteString(" ")
			}
			row.WriteString(hourCell(g.Cells[wd][h], g.MaxCell))
		}
		b.WriteString(row.String() + "\n")
	}

	b.WriteString("\nTotal: " + styleTime.Render(fmtDur(g.Total)) +
		" across " + styleTime.Render(formatCount(g.Count)) +
		" · Peak: " + styleTime.Render(weekdayNames[g.PeakWD]+" "+fmt.Sprintf("%02d:00", g.PeakHour)) + "\n")
	return b.String()
}

func formatDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}
