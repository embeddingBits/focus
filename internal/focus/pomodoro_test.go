package focus

import (
	"testing"
	"time"
)

func TestDefaultPomodoroConfig(t *testing.T) {
	cfg := DefaultPomodoroConfig()
	if cfg.Work != 25*time.Minute {
		t.Fatalf("Work = %v, want 25m", cfg.Work)
	}
	if cfg.ShortBreak != 5*time.Minute {
		t.Fatalf("ShortBreak = %v, want 5m", cfg.ShortBreak)
	}
	if cfg.LongBreak != 15*time.Minute {
		t.Fatalf("LongBreak = %v, want 15m", cfg.LongBreak)
	}
	if cfg.BlocksBeforeLong != 4 {
		t.Fatalf("BlocksBeforeLong = %d, want 4", cfg.BlocksBeforeLong)
	}
}

func TestNewPomodoroCycleSanitizes(t *testing.T) {
	fc := &fakeClock{now: time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)}
	c := NewPomodoroCycle(PomodoroConfig{}, fc)
	if c.Phase() != PomodoroWork {
		t.Fatalf("initial phase = %q, want work", c.Phase())
	}
	if c.PhaseDuration() != 25*time.Minute {
		t.Fatalf("PhaseDuration = %v, want 25m", c.PhaseDuration())
	}
	if c.BlocksPerSet() != 4 {
		t.Fatalf("BlocksPerSet = %d, want 4", c.BlocksPerSet())
	}
	if c == nil {
		t.Fatal("want non-nil cycle")
	}
}

// TestPomodoroRotation drives a full work→short→…→long→work rotation with a
// fake clock: each row advances the clock past the current phase, calls
// Advance, and checks the new phase, its duration, counts, and label.
func TestPomodoroRotation(t *testing.T) {
	cfg := PomodoroConfig{
		Work:             25 * time.Minute,
		ShortBreak:       5 * time.Minute,
		LongBreak:        15 * time.Minute,
		BlocksBeforeLong: 4,
	}
	steps := []struct {
		phase     PomodoroPhase
		duration  time.Duration
		completed int
		block     int
		label     string
	}{
		{PomodoroWork, 25 * time.Minute, 0, 1, "Work 1 of 4"},
		{PomodoroShortBreak, 5 * time.Minute, 1, 2, "Short break"},
		{PomodoroWork, 25 * time.Minute, 1, 2, "Work 2 of 4"},
		{PomodoroShortBreak, 5 * time.Minute, 2, 3, "Short break"},
		{PomodoroWork, 25 * time.Minute, 2, 3, "Work 3 of 4"},
		{PomodoroShortBreak, 5 * time.Minute, 3, 4, "Short break"},
		{PomodoroWork, 25 * time.Minute, 3, 4, "Work 4 of 4"},
		{PomodoroLongBreak, 15 * time.Minute, 4, 1, "Long break"},
		{PomodoroWork, 25 * time.Minute, 0, 1, "Work 1 of 4"},
	}

	fc := &fakeClock{now: time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)}
	c := NewPomodoroCycle(cfg, fc)
	for i, want := range steps {
		if got := c.Phase(); got != want.phase {
			t.Fatalf("step %d: phase = %q, want %q", i, got, want.phase)
		}
		if got := c.PhaseDuration(); got != want.duration {
			t.Fatalf("step %d: duration = %v, want %v", i, got, want.duration)
		}
		if got := c.WorkCompleted(); got != want.completed {
			t.Fatalf("step %d: completed = %d, want %d", i, got, want.completed)
		}
		if got := c.CurrentBlock(); got != want.block {
			t.Fatalf("step %d: block = %d, want %d", i, got, want.block)
		}
		if got := c.PhaseLabel(); got != want.label {
			t.Fatalf("step %d: label = %q, want %q", i, got, want.label)
		}
		// Elapsed/Remaining are clock-driven per phase: halfway through the
		// phase, remaining must be half the duration.
		half := want.duration / 2
		fc.advance(half)
		if got := c.Elapsed(fc.Now()); got != half {
			t.Fatalf("step %d: elapsed = %v, want %v", i, got, half)
		}
		if got := c.Remaining(fc.Now()); got != want.duration-half {
			t.Fatalf("step %d: remaining = %v, want %v", i, got, want.duration-half)
		}
		fc.advance(half) // reach the phase end
		c.Advance()
	}
}

// TestPomodoroCustomSetSize checks the long-break cadence follows
// BlocksBeforeLong (here: every 2 work blocks).
func TestPomodoroCustomSetSize(t *testing.T) {
	cfg := PomodoroConfig{
		Work:             20 * time.Minute,
		ShortBreak:       3 * time.Minute,
		LongBreak:        10 * time.Minute,
		BlocksBeforeLong: 2,
	}
	fc := &fakeClock{now: time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)}
	c := NewPomodoroCycle(cfg, fc)

	want := []PomodoroPhase{
		PomodoroWork, PomodoroShortBreak,
		PomodoroWork, PomodoroLongBreak,
		PomodoroWork,
	}
	for i, phase := range want {
		if got := c.Phase(); got != phase {
			t.Fatalf("step %d: phase = %q, want %q", i, got, phase)
		}
		c.Advance()
	}
	// After the long break the set resets: work 1 of the new set completes
	// into a short break with block 2 upcoming.
	if got := c.Phase(); got != PomodoroShortBreak {
		t.Fatalf("phase after second-set work 1 = %q, want short-break", got)
	}
	if got := c.CurrentBlock(); got != 2 {
		t.Fatalf("block after second-set work 1 = %d, want 2", got)
	}
	if got := c.WorkCompleted(); got != 1 {
		t.Fatalf("completed after long break + 1 work = %d, want 1 (set reset)", got)
	}
}

func TestPomodoroNilClockUsesWall(t *testing.T) {
	c := NewPomodoroCycle(DefaultPomodoroConfig(), nil)
	if c.Phase() != PomodoroWork {
		t.Fatalf("phase = %q, want work", c.Phase())
	}
	if got := c.PhaseDuration(); got != 25*time.Minute {
		t.Fatalf("duration = %v, want 25m", got)
	}
}
