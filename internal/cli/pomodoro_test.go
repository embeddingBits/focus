package cli

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/focus-cli/focus/internal/focus"
	"github.com/focus-cli/focus/internal/tui"
)

// fakeStore is an in-memory focus.Store: rotation/persistence mapping without SQL.
type fakeStore struct {
	mu      sync.Mutex
	records []focus.SessionRecord
	nextID  int64
}

func (s *fakeStore) Migrate(ctx context.Context) error { return nil }
func (s *fakeStore) Close() error                      { return nil }

func (s *fakeStore) Create(ctx context.Context, r focus.SessionRecord) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r.ID = s.nextID
	s.records = append(s.records, r)
	return r.ID, nil
}

func (s *fakeStore) Complete(ctx context.Context, id int64, endedAt time.Time, pausedSeconds int64, accomplishment, next string, completed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.records {
		if r.ID == id {
			s.records[i].EndedAt = &endedAt
			s.records[i].PausedSeconds = pausedSeconds
			s.records[i].Accomplishment = accomplishment
			s.records[i].Next = next
			s.records[i].Completed = completed
			return nil
		}
	}
	return errors.New("no such session")
}

func (s *fakeStore) List(ctx context.Context, limit int) ([]focus.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]focus.SessionRecord(nil), s.records...), nil
}

func (s *fakeStore) StatsToday(ctx context.Context, now time.Time) (time.Duration, int, time.Duration, error) {
	return 0, 0, 0, nil
}

// scriptRunner replays canned results while recording the requests it saw.
type scriptRunner struct {
	results []tui.TimerResult
	seen    []tui.TimerRequest
}

func (r *scriptRunner) run(ctx context.Context, req tui.TimerRequest) (tui.TimerResult, error) {
	r.seen = append(r.seen, req)
	if len(r.results) == 0 {
		return tui.TimerResult{Completed: true}, nil
	}
	res := r.results[0]
	r.results = r.results[1:]
	return res, nil
}

func testPomoConfig() focus.PomodoroConfig {
	return focus.PomodoroConfig{
		Work:             25 * time.Minute,
		ShortBreak:       5 * time.Minute,
		LongBreak:        15 * time.Minute,
		BlocksBeforeLong: 4,
	}
}

func TestRunPomodoroPersistsWorkBlocks(t *testing.T) {
	store := &fakeStore{}
	runner := &scriptRunner{results: []tui.TimerResult{
		{Completed: true, Accomplishment: "did w1", Next: "w2"},
		{Completed: true}, // short break elapsed
		{Completed: true, Accomplishment: "did w2", Next: "w3"},
	}}
	var out bytes.Buffer
	done, err := runPomodoro(context.Background(), store, &out, "write report", testPomoConfig(), 2, nil, runner.run)
	if err != nil {
		t.Fatalf("runPomodoro: %v", err)
	}
	if done != 2 {
		t.Fatalf("done = %d, want 2", done)
	}
	if len(store.records) != 2 {
		t.Fatalf("stored rows = %d, want 2 (one per work block)", len(store.records))
	}
	for i, r := range store.records {
		if !r.Completed {
			t.Fatalf("row %d Completed = false, want true", i)
		}
		if r.Task != "write report" {
			t.Fatalf("row %d task = %q, want untouched task", i, r.Task)
		}
		if r.PlannedSeconds != 1500 {
			t.Fatalf("row %d planned = %d, want 1500", i, r.PlannedSeconds)
		}
		if r.EndedAt == nil {
			t.Fatalf("row %d has no EndedAt", i)
		}
	}
	if store.records[0].Accomplishment != "did w1" {
		t.Fatalf("row 0 accomplishment = %q, want carried prompt", store.records[0].Accomplishment)
	}
	if len(runner.seen) != 3 {
		t.Fatalf("timer runs = %d, want 3 (work, break, work)", len(runner.seen))
	}
	if runner.seen[0].PhaseLabel != "Work 1 of 4" || runner.seen[0].Break {
		t.Fatalf("run 0 = %+v, want work label, not break", runner.seen[0])
	}
	if runner.seen[1].PhaseLabel != "Short break" || !runner.seen[1].Break {
		t.Fatalf("run 1 = %+v, want skippable short break", runner.seen[1])
	}
	if runner.seen[1].Planned != 5*time.Minute {
		t.Fatalf("break planned = %v, want 5m", runner.seen[1].Planned)
	}
	if runner.seen[2].PhaseLabel != "Work 2 of 4" || runner.seen[2].Break {
		t.Fatalf("run 2 = %+v, want work 2 of 4", runner.seen[2])
	}
}

func TestRunPomodoroLongBreakCadence(t *testing.T) {
	cfg := testPomoConfig()
	cfg.BlocksBeforeLong = 2
	store := &fakeStore{}
	runner := &scriptRunner{} // defaults: everything completes
	var out bytes.Buffer
	done, err := runPomodoro(context.Background(), store, &out, "task", cfg, 3, nil, runner.run)
	if err != nil {
		t.Fatalf("runPomodoro: %v", err)
	}
	if done != 3 {
		t.Fatalf("done = %d, want 3", done)
	}
	want := []struct {
		label string
		isBrk bool
	}{
		{"Work 1 of 2", false},
		{"Short break", true},
		{"Work 2 of 2", false},
		{"Long break", true},
		{"Work 1 of 2", false},
	}
	if len(runner.seen) != len(want) {
		t.Fatalf("timer runs = %d, want %d", len(runner.seen), len(want))
	}
	for i, w := range want {
		if runner.seen[i].PhaseLabel != w.label || runner.seen[i].Break != w.isBrk {
			t.Fatalf("run %d = %+v, want label %q break=%v", i, runner.seen[i], w.label, w.isBrk)
		}
	}
	if runner.seen[3].Planned != 15*time.Minute {
		t.Fatalf("long break planned = %v, want 15m", runner.seen[3].Planned)
	}
}

func TestRunPomodoroAbandonStopsRotation(t *testing.T) {
	store := &fakeStore{}
	runner := &scriptRunner{results: []tui.TimerResult{{Completed: false}}}
	var out bytes.Buffer
	done, err := runPomodoro(context.Background(), store, &out, "task", testPomoConfig(), 0, nil, runner.run)
	if err != nil {
		t.Fatalf("runPomodoro: %v", err)
	}
	if done != 0 {
		t.Fatalf("done = %d, want 0", done)
	}
	if len(store.records) != 1 || store.records[0].Completed {
		t.Fatalf("abandoned block must persist as one Completed=false row: %+v", store.records)
	}
	if len(runner.seen) != 1 {
		t.Fatalf("timer runs = %d, want 1 (rotation stops after abandon)", len(runner.seen))
	}
}

func TestRunPomodoroQuitBreakStopsRotation(t *testing.T) {
	store := &fakeStore{}
	runner := &scriptRunner{results: []tui.TimerResult{
		{Completed: true, Accomplishment: "did it"},
		{Completed: false}, // quit during the break
	}}
	var out bytes.Buffer
	done, err := runPomodoro(context.Background(), store, &out, "task", testPomoConfig(), 0, nil, runner.run)
	if err != nil {
		t.Fatalf("runPomodoro: %v", err)
	}
	if done != 1 {
		t.Fatalf("done = %d, want 1", done)
	}
	if len(store.records) != 1 {
		t.Fatalf("stored rows = %d, want 1 (breaks are never persisted)", len(store.records))
	}
}

func TestPomodoroFlagValidation(t *testing.T) {
	for _, args := range [][]string{
		{"task", "--blocks", "-1"},
		{"task", "--work", "-5"},
		{"task", "--short-break", "0"},
		{"task", "--long-break", "-1"},
		{"task", "--every", "-2"},
	} {
		cmd := newPomodoroCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		var ue UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("args %v: err = %v, want UsageError (exit 2)", args, err)
		}
		if ExitCode(err) != 2 {
			t.Fatalf("args %v: exit = %d, want 2", args, ExitCode(err))
		}
	}
}
