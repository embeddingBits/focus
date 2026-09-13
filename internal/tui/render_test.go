package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/focus-cli/focus/internal/focus"
	"github.com/muesli/termenv"
)

// TestMain pins the color profile so golden strings are deterministic
// with or without a TTY.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func timePtr(t time.Time) *time.Time { return &t }

func fixtureRecords() (done1, done2, abandoned focus.SessionRecord) {
	done1 = focus.SessionRecord{
		ID:             1,
		Task:           "write report",
		PlannedSeconds: 1500,
		StartedAt:      time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC),
		EndedAt:        timePtr(time.Date(2026, 9, 13, 14, 25, 0, 0, time.UTC)),
		PausedSeconds:  60,
		Accomplishment: "shipped draft",
		Next:           "send invoice",
		Completed:      true,
	}
	done2 = focus.SessionRecord{
		ID:             2,
		Task:           "review PR",
		PlannedSeconds: 1500,
		StartedAt:      time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC),
		EndedAt:        timePtr(time.Date(2026, 9, 13, 10, 20, 0, 0, time.UTC)),
		Completed:      true,
	}
	abandoned = focus.SessionRecord{
		ID:             3,
		Task:           "read inbox",
		PlannedSeconds: 1500,
		StartedAt:      time.Date(2026, 9, 13, 9, 30, 0, 0, time.UTC),
		EndedAt:        timePtr(time.Date(2026, 9, 13, 9, 35, 0, 0, time.UTC)),
		Completed:      false,
	}
	return done1, done2, abandoned
}

func TestRenderHistoryGolden(t *testing.T) {
	done1, done2, abandoned := fixtureRecords()
	got := RenderHistory([]focus.SessionRecord{done1, done2, abandoned}, 10)

	want := "History\n" +
		"2026-09-13 14:00  write report  25m → 24m  completed\n" +
		"    did: shipped draft · next: send invoice\n" +
		"2026-09-13 10:00  review PR  25m → 20m  completed\n" +
		"2026-09-13 09:30  read inbox  25m → 5m  abandoned\n"

	if got != want {
		t.Fatalf("RenderHistory mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderHistoryLimit(t *testing.T) {
	done1, done2, abandoned := fixtureRecords()
	got := RenderHistory([]focus.SessionRecord{done1, done2, abandoned}, 1)
	if strings.Contains(got, "review PR") || strings.Contains(got, "read inbox") {
		t.Fatalf("limit=1 should truncate to first record:\n%s", got)
	}
	if !strings.Contains(got, "write report") {
		t.Fatalf("limit=1 should keep first record:\n%s", got)
	}
}

func TestRenderHistoryEmpty(t *testing.T) {
	got := RenderHistory(nil, 20)
	if !strings.Contains(got, "No sessions yet") {
		t.Fatalf("empty history should say so:\n%s", got)
	}
}

func TestRenderHistoryInProgress(t *testing.T) {
	r := focus.SessionRecord{
		ID:             7,
		Task:           "still going",
		PlannedSeconds: 1500,
		StartedAt:      time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC),
		EndedAt:        nil,
	}
	got := RenderHistory([]focus.SessionRecord{r}, 10)
	if !strings.Contains(got, "in progress") {
		t.Fatalf("nil EndedAt should render in progress:\n%s", got)
	}
}

func TestRenderStatsGolden(t *testing.T) {
	done1, done2, _ := fixtureRecords()
	got := RenderStats(44*time.Minute, 2, 22*time.Minute, []focus.SessionRecord{done1, done2})

	want := "Stats\n" +
		"Today: 44m across 2 sessions (avg 22m)\n\n" +
		"2026-09-13 14:00  write report  25m → 24m  completed\n" +
		"2026-09-13 10:00  review PR  25m → 20m  completed\n"

	if got != want {
		t.Fatalf("RenderStats mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderStatsEmpty(t *testing.T) {
	got := RenderStats(0, 0, 0, nil)
	if !strings.Contains(got, "No completed sessions today") {
		t.Fatalf("zero-count stats should say so:\n%s", got)
	}
}

func TestFmtHMS(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "00:00:00",
		90 * time.Second: "00:01:30",
		25 * time.Minute: "00:25:00",
		2*time.Hour + 5*time.Minute + 7*time.Second: "02:05:07",
		-3 * time.Second: "00:00:00",
	}
	for d, want := range cases {
		if got := fmtHMS(d); got != want {
			t.Errorf("fmtHMS(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestFmtDur(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Second:            "45s",
		25 * time.Minute:            "25m",
		90 * time.Second:            "1m30s",
		2*time.Hour + 5*time.Minute: "2h5m",
		0:                           "0s",
	}
	for d, want := range cases {
		if got := fmtDur(d); got != want {
			t.Errorf("fmtDur(%v) = %q, want %q", d, got, want)
		}
	}
}
