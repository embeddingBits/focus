package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

func heatFixture(now time.Time) []focus.SessionRecord {
	end := func(t time.Time) *time.Time { return &t }
	return []focus.SessionRecord{
		{
			ID: 1, Task: "deep work", PlannedSeconds: 1500,
			StartedAt: now.Add(-2 * time.Hour), EndedAt: end(now.Add(-90 * time.Minute)),
			Completed: true,
		},
		{
			ID: 2, Task: "review", PlannedSeconds: 1500,
			StartedAt: now.Add(-50 * time.Hour), EndedAt: end(now.Add(-50*time.Hour + 10*time.Minute)),
			Completed: true,
		},
		{
			ID: 3, Task: "abandoned", PlannedSeconds: 1500,
			StartedAt: now, EndedAt: end(now.Add(5 * time.Minute)),
			Completed: false,
		},
		{
			ID: 4, Task: "rest", Kind: focus.KindBreak, PlannedSeconds: 300,
			StartedAt: now, EndedAt: end(now.Add(5 * time.Minute)),
			Completed: true,
		},
	}
}

func TestBuildWeekHeatBuckets(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	days := BuildWeekHeat(heatFixture(now), now)
	if len(days) != 7 {
		t.Fatalf("BuildWeekHeat len = %d, want 7", len(days))
	}
	// Today holds only the 30m deep-work session; break + abandoned excluded.
	today := days[6]
	if today.Count != 1 || today.Total != 30*time.Minute {
		t.Fatalf("today = %+v, want count 1 total 30m", today)
	}
	// Two days ago holds the 10m review session.
	twoAgo := days[4]
	if twoAgo.Count != 1 || twoAgo.Total != 10*time.Minute {
		t.Fatalf("two days ago = %+v, want count 1 total 10m", twoAgo)
	}
	// Other days empty.
	for i, d := range days {
		if i == 6 || i == 4 {
			continue
		}
		if d.Count != 0 || d.Total != 0 {
			t.Fatalf("day %d = %+v, want empty", i, d)
		}
	}
	// Oldest-first ordering.
	for i := 1; i < len(days); i++ {
		if !days[i].Date.After(days[i-1].Date) {
			t.Fatalf("days not oldest-first: %v then %v", days[i-1].Date, days[i].Date)
		}
	}
}

func TestRenderHeatmapGolden(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	days := BuildWeekHeat(heatFixture(now), now)
	got := RenderHeatmap(days)

	if !strings.Contains(got, "Heatmap") {
		t.Fatalf("heatmap should have header:\n%s", got)
	}
	// 30m today -> ▒, 10m two days ago -> ░, empties -> ·
	for _, want := range []string{"▒", "░", "·"} {
		if !strings.Contains(got, want) {
			t.Fatalf("heatmap should contain %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "Week: 40m across 2 sessions") {
		t.Fatalf("week summary missing:\n%s", got)
	}
	if strings.Contains(got, "abandoned") || strings.Contains(got, "rest") {
		t.Fatalf("heatmap must not list excluded rows:\n%s", got)
	}
}

func TestRenderHeatmapEmpty(t *testing.T) {
	got := RenderHeatmap(nil)
	if !strings.Contains(got, "No completed sessions this week") {
		t.Fatalf("empty heatmap should say so:\n%s", got)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	got = RenderHeatmap(BuildWeekHeat(nil, now))
	if !strings.Contains(got, "No completed sessions this week") {
		t.Fatalf("all-empty week should say so:\n%s", got)
	}
}

func TestHeatBlockThresholds(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "·",
		5 * time.Minute:  "░",
		30 * time.Minute: "▒",
		90 * time.Minute: "▓",
		3 * time.Hour:    "█",
		-1 * time.Minute: "·",
	}
	for d, want := range cases {
		if got := heatBlock(d); got != want {
			t.Errorf("heatBlock(%v) = %q, want %q", d, got, want)
		}
	}
}
