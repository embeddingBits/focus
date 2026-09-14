package storage

import (
	"context"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
)

func openTestStore(t *testing.T) (*SQLiteStore, context.Context) {
	t.Helper()
	ctx := t.Context()
	s, err := Open(t.TempDir() + "/focus.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s, ctx
}

func TestMigrateIdempotent(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if count != 2 {
		t.Fatalf("applied migrations = %d, want 2", count)
	}
}

func TestCreateCompleteListRoundTrip(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	started := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	id, err := s.Create(ctx, focus.SessionRecord{
		Task:           "write report",
		PlannedSeconds: 1500,
		StartedAt:      started,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Fatalf("Create id = %d, want > 0", id)
	}
	ended := started.Add(25 * time.Minute)
	if err := s.Complete(ctx, id, ended, 60, "shipped it", "rest", true); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	records, err := s.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("List len = %d, want 1", len(records))
	}
	got := records[0]
	if got.ID != id || got.Task != "write report" || got.PlannedSeconds != 1500 {
		t.Fatalf("identity fields wrong: %+v", got)
	}
	if !got.StartedAt.Equal(started) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, started)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(ended) {
		t.Fatalf("EndedAt = %v, want %v", got.EndedAt, ended)
	}
	if got.PausedSeconds != 60 || got.Accomplishment != "shipped it" || got.Next != "rest" {
		t.Fatalf("completed fields wrong: %+v", got)
	}
	if !got.Completed {
		t.Fatal("Completed = false, want true")
	}
}

func TestCreateDefaultsEmptyAccomplishment(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	id, err := s.Create(ctx, focus.SessionRecord{
		Task:           "quick",
		PlannedSeconds: 60,
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Complete(ctx, id, time.Now(), 0, "", "", true); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	records, err := s.List(ctx, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 || records[0].Accomplishment != "" || records[0].Next != "" {
		t.Fatalf("empty accomplishment/next not preserved: %+v", records)
	}
}

func TestCompleteUnknownID(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := s.Complete(ctx, 999, time.Now(), 0, "", "", true); err == nil {
		t.Fatal("Complete unknown id: want error, got nil")
	}
}

func TestStatsToday(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// now in a fixed zone so local-midnight bounds are deterministic.
	loc := time.FixedZone("UTC+2", 2*3600)
	now := time.Date(2026, 9, 13, 15, 0, 0, 0, loc)

	// Two completed sessions today: 25m-1m pause = 24m, 10m exact.
	// One abandoned session today (excluded). One completed yesterday (excluded).
	fixtures := []struct {
		started      time.Time
		elapsed      time.Duration
		paused       int64
		completed    bool
		accomplished bool
	}{
		{time.Date(2026, 9, 13, 9, 0, 0, 0, loc), 25 * time.Minute, 60, true, true},
		{time.Date(2026, 9, 13, 12, 0, 0, 0, loc), 10 * time.Minute, 0, true, true},
		{time.Date(2026, 9, 13, 13, 0, 0, 0, loc), 10 * time.Minute, 0, false, false},
		{time.Date(2026, 9, 12, 16, 0, 0, 0, loc), 50 * time.Minute, 0, true, true},
	}
	for i, f := range fixtures {
		id, err := s.Create(ctx, focus.SessionRecord{
			Task:           "task",
			PlannedSeconds: int64(f.elapsed / time.Second),
			StartedAt:      f.started,
		})
		if err != nil {
			t.Fatalf("fixture %d Create: %v", i, err)
		}
		if err := s.Complete(ctx, id, f.started.Add(f.elapsed), f.paused, "", "", f.completed); err != nil {
			t.Fatalf("fixture %d Complete: %v", i, err)
		}
	}

	total, count, avg, err := s.StatsToday(ctx, now)
	if err != nil {
		t.Fatalf("StatsToday: %v", err)
	}
	wantTotal := 24*time.Minute + 10*time.Minute
	if total != wantTotal {
		t.Fatalf("total = %v, want %v", total, wantTotal)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if avg != wantTotal/2 {
		t.Fatalf("avg = %v, want %v", avg, wantTotal/2)
	}
}

func TestStatsTodayEmpty(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	total, count, avg, err := s.StatsToday(ctx, time.Now())
	if err != nil {
		t.Fatalf("StatsToday: %v", err)
	}
	if total != 0 || count != 0 || avg != 0 {
		t.Fatalf("empty stats = (%v, %d, %v), want zeros", total, count, avg)
	}
}

func TestListNewestFirst(t *testing.T) {
	s, ctx := openTestStore(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	base := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	for i, task := range []string{"first", "second", "third"} {
		if _, err := s.Create(ctx, focus.SessionRecord{
			Task:           task,
			PlannedSeconds: 60,
			StartedAt:      base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatalf("Create %q: %v", task, err)
		}
	}
	records, err := s.List(ctx, 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("List len = %d, want 2 (limit)", len(records))
	}
	if records[0].Task != "third" || records[1].Task != "second" {
		t.Fatalf("order wrong: %q, %q", records[0].Task, records[1].Task)
	}
}
