package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
	"github.com/focus-cli/focus/internal/storage"
)

func seedCompletedSession(t *testing.T, ctx context.Context, store *storage.SQLiteStore, task string, start time.Time, length time.Duration) {
	t.Helper()
	id, err := store.Create(ctx, focus.SessionRecord{
		Task:           task,
		PlannedSeconds: int64(length / time.Second),
		StartedAt:      start,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Complete(ctx, id, start.Add(length), 0, "", "", true); err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func runHeatmap(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newHeatmapCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestHeatmapCmdShowsGrid(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)

	store, err := storage.Open(dir + "/focus.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now()
	seedCompletedSession(t, ctx, store, "deep work", now.Add(-30*time.Minute), 25*time.Minute)
	store.Close()

	got, err := runHeatmap(t)
	if err != nil {
		t.Fatalf("heatmap execute: %v", err)
	}
	for _, want := range []string{
		"Heatmap",
		"When are you most productive?",
		"Mon Tue Wed Thu Fri Sat Sun",
		"Peak:",
		"1 session",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("heatmap output should contain %q:\n%s", want, got)
		}
	}
}

func TestHeatmapCmdEmptyWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)

	got, err := runHeatmap(t)
	if err != nil {
		t.Fatalf("heatmap execute: %v", err)
	}
	if !strings.Contains(got, "No completed sessions in the last 30 days") {
		t.Fatalf("empty heatmap should say so:\n%s", got)
	}
}

func TestHeatmapCmdBadDays(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)

	if _, err := runHeatmap(t, "--days", "0"); err == nil {
		t.Fatal("--days 0 should fail")
	} else if code := ExitCode(err); code != 2 {
		t.Fatalf("--days 0 exit = %d, want 2 (usage)", code)
	}
}
