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

func TestHeatmapCmdShowsWeek(t *testing.T) {
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

	cmd := newHeatmapCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("heatmap execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Heatmap") {
		t.Fatalf("heatmap output should have header:\n%s", got)
	}
	if !strings.Contains(got, "Week:") {
		t.Fatalf("heatmap output should summarize the week:\n%s", got)
	}
	if !strings.Contains(got, "1 session") {
		t.Fatalf("heatmap output should count the seeded session:\n%s", got)
	}
}

func TestHeatmapCmdEmptyWeek(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOCUS_DATA_DIR", dir)

	cmd := newHeatmapCmd()
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("heatmap execute: %v", err)
	}
	if !strings.Contains(out.String(), "No completed sessions this week") {
		t.Fatalf("empty heatmap should say so:\n%s", out.String())
	}
}
