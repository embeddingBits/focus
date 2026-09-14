package cli

import (
	"context"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
	"github.com/focus-cli/focus/internal/storage"
	"github.com/focus-cli/focus/internal/tui"
)

func TestBreakDefaultFromEnv(t *testing.T) {
	t.Setenv("FOCUS_BREAK_SECONDS", "")
	if got := breakDefaultFromEnv(); got != 0 {
		t.Fatalf("empty env = %v, want 0 (TUI default)", got)
	}
	t.Setenv("FOCUS_BREAK_SECONDS", "3")
	if got := breakDefaultFromEnv(); got != 3*time.Second {
		t.Fatalf("env 3 = %v, want 3s", got)
	}
	for _, bad := range []string{"0", "-5", "abc", "1.5"} {
		t.Setenv("FOCUS_BREAK_SECONDS", bad)
		if got := breakDefaultFromEnv(); got != 0 {
			t.Fatalf("env %q = %v, want 0 fallback", bad, got)
		}
	}
}

func TestBreakPersistHookWritesKindBreakRow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)

	store, err := storage.Open(dir + "/focus.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	start := time.Now().Add(-time.Minute).UTC()
	hook := breakPersistHook(ctx, store, "write report")
	hook(tui.BreakInfo{
		Planned:   5 * time.Minute,
		Taken:     time.Minute,
		StartedAt: start,
		EndedAt:   start.Add(time.Minute),
	})

	rows, err := store.List(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 persisted break row, got %d", len(rows))
	}
	r := rows[0]
	if r.Kind != focus.KindBreak {
		t.Fatalf("Kind = %q, want %q", r.Kind, focus.KindBreak)
	}
	if !r.Completed {
		t.Fatal("break row must be Completed")
	}
	if r.Task != "write report" {
		t.Fatalf("Task = %q", r.Task)
	}
	if r.PlannedSeconds != 300 {
		t.Fatalf("PlannedSeconds = %d, want 300", r.PlannedSeconds)
	}

	total, count, _, err := store.StatsToday(ctx, time.Now())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if count != 0 || total != 0 {
		t.Fatalf("break must stay out of stats, got total=%v count=%d", total, count)
	}
}
