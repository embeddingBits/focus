package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

// mondayNoon anchors fixtures to a known Monday (2026-09-14) in UTC.
func mondayNoon() time.Time {
	return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
}

func gridFixture() []focus.SessionRecord {
	end := func(t time.Time) *time.Time { return &t }
	mon := mondayNoon()
	tue := mon.AddDate(0, 0, 1)
	return []focus.SessionRecord{
		{
			ID: 1, Task: "deep work", PlannedSeconds: 1500,
			StartedAt: time.Date(2026, 9, 14, 8, 10, 0, 0, time.UTC),
			EndedAt:   end(time.Date(2026, 9, 14, 8, 40, 0, 0, time.UTC)),
			Completed: true,
		},
		{
			// Spans the 09:00 boundary: 10m land in 09, 20m in 10 (Tuesday).
			ID: 2, Task: "review", PlannedSeconds: 1800,
			StartedAt: time.Date(2026, 9, 15, 9, 50, 0, 0, time.UTC),
			EndedAt:   end(time.Date(2026, 9, 15, 10, 20, 0, 0, time.UTC)),
			Completed: true,
		},
		{
			// Paused 10 of 30 wall minutes: 20m honest, all Monday 11.
			ID: 3, Task: "write", PlannedSeconds: 1800,
			StartedAt:     time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC),
			EndedAt:       end(time.Date(2026, 9, 14, 11, 30, 0, 0, time.UTC)),
			PausedSeconds: 600,
			Completed:     true,
		},
		{
			ID: 4, Task: "abandoned", PlannedSeconds: 1500,
			StartedAt: mon, EndedAt: end(mon.Add(5 * time.Minute)),
			Completed: false,
		},
		{
			ID: 5, Task: "rest", Kind: focus.KindBreak, PlannedSeconds: 300,
			StartedAt: tue, EndedAt: end(tue.Add(5 * time.Minute)),
			Completed: true,
		},
		{
			// Outside a 30-day window ending 2026-09-15.
			ID: 6, Task: "ancient", PlannedSeconds: 1500,
			StartedAt: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
			EndedAt:   end(time.Date(2026, 7, 1, 9, 25, 0, 0, time.UTC)),
			Completed: true,
		},
	}
}

func TestBuildProductivityGridBuckets(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	g := BuildProductivityGrid(gridFixture(), now, 30)

	if g.Count != 3 {
		t.Fatalf("Count = %d, want 3 (break, abandoned, ancient excluded)", g.Count)
	}
	if g.Cells[0][8] != 30*time.Minute {
		t.Fatalf("Mon 08 = %v, want 30m", g.Cells[0][8])
	}
	if g.Cells[1][9] != 10*time.Minute {
		t.Fatalf("Tue 09 = %v, want 10m", g.Cells[1][9])
	}
	if g.Cells[1][10] != 20*time.Minute {
		t.Fatalf("Tue 10 = %v, want 20m", g.Cells[1][10])
	}
	if g.Cells[0][11] != 20*time.Minute {
		t.Fatalf("Mon 11 = %v, want 20m honest (30 wall minus 10 paused)", g.Cells[0][11])
	}
	if g.Total != 80*time.Minute {
		t.Fatalf("Total = %v, want 80m", g.Total)
	}
	if g.MinHour != 8 || g.MaxHour != 11 {
		t.Fatalf("hour range = %d to %d, want 8 to 11", g.MinHour, g.MaxHour)
	}
	if g.PeakWD != 0 || g.PeakHour != 8 || g.Peak != 30*time.Minute {
		t.Fatalf("Peak = %s %d:00 %v, want Mon 8:00 30m", weekdayNames[g.PeakWD], g.PeakHour, g.Peak)
	}
}

func TestBuildProductivityGridEmpty(t *testing.T) {
	now := mondayNoon()
	g := BuildProductivityGrid(nil, now, 30)
	if g.MaxCell != 0 || g.Count != 0 || g.Total != 0 {
		t.Fatalf("empty grid = %+v, want zeros", g)
	}
	if got := RenderProductivityGrid(g); !strings.Contains(got, "No completed sessions in the last 30 days") {
		t.Fatalf("empty grid should say so:\n%s", got)
	}
}

func TestRenderProductivityGridGolden(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	got := RenderProductivityGrid(BuildProductivityGrid(gridFixture(), now, 30))

	for _, want := range []string{
		"Heatmap",
		"When are you most productive?",
		"Mon Tue Wed Thu Fri Sat Sun",
		"08:00",
		"11:00",
		"■", // active cells are filled squares
		"□", // empty weekend cells are outlines
		"Peak: Mon 08:00",
		"Total: 1h20m across 3 sessions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("heatmap should contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "abandoned") || strings.Contains(got, "ancient") {
		t.Errorf("heatmap must not list excluded rows:\n%s", got)
	}
	// Trace cell: Tue 09 holds 10m of a 30m max, so it must not be blank.
	lines := strings.Split(got, "\n")
	var nine string
	for _, l := range lines {
		if strings.HasPrefix(l, "09:00") {
			nine = l
		}
	}
	if nine == "" {
		t.Fatalf("heatmap should have a 09:00 row:\n%s", got)
	}
	if !strings.Contains(nine, "■") {
		t.Errorf("09:00 row should show Tue activity:\n%s", nine)
	}
}

func TestHeatLevel(t *testing.T) {
	max := 120 * time.Minute
	cases := map[time.Duration]int{
		0:                 0,
		-5 * time.Minute:  0,
		time.Minute:       1,
		30 * time.Minute:  1, // exactly the first quartile
		60 * time.Minute:  2,
		90 * time.Minute:  3,
		120 * time.Minute: 4,
		180 * time.Minute: 4, // clamped, never above 4
	}
	for d, want := range cases {
		if got := heatLevel(d, max); got != want {
			t.Errorf("heatLevel(%v) = %d, want %d", d, got, want)
		}
	}
	if got := heatLevel(10*time.Minute, 0); got != 0 {
		t.Errorf("heatLevel with zero max = %d, want 0", got)
	}
}

func TestHourCell(t *testing.T) {
	max := 120 * time.Minute
	if got := hourCell(0, max); got != "□" {
		t.Errorf("hourCell(0) = %q, want outline square", got)
	}
	if got := hourCell(max, max); got != "■" {
		t.Errorf("hourCell(max) = %q, want filled square", got)
	}
}
