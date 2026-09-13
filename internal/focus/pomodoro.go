// Package focus — pomodoro cycle model.
//
// PomodoroCycle is the work/break rotation on top of the Phase 1 timer:
// repeating work blocks with short breaks and a long break after every
// BlocksBeforeLong work blocks. Like Session it is pure (no I/O) and
// clock-injected; the CLI drives one phase per timer run and calls Advance
// when the phase ends. Breaks are never persisted — each work block
// completes as a normal session row via the Store port.
package focus

import (
	"fmt"
	"time"
)

// Classic pomodoro defaults: 25-minute work blocks, 5-minute short breaks,
// a 15-minute long break after every 4 work blocks.
const (
	DefaultPomodoroWork       = 25 * time.Minute
	DefaultPomodoroShortBreak = 5 * time.Minute
	DefaultPomodoroLongBreak  = 15 * time.Minute
	// DefaultBlocksBeforeLong is how many work blocks form one set.
	DefaultBlocksBeforeLong = 4
)

// PomodoroPhase is one step of the rotation.
type PomodoroPhase string

const (
	// PomodoroWork is a focused work block (persisted as a session row).
	PomodoroWork PomodoroPhase = "work"
	// PomodoroShortBreak is the rest between work blocks in a set.
	PomodoroShortBreak PomodoroPhase = "short-break"
	// PomodoroLongBreak is the rest after every BlocksBeforeLong work blocks.
	PomodoroLongBreak PomodoroPhase = "long-break"
)

// PomodoroConfig tunes the rotation. Non-positive values select the classic
// defaults (see DefaultPomodoroConfig).
type PomodoroConfig struct {
	Work             time.Duration
	ShortBreak       time.Duration
	LongBreak        time.Duration
	BlocksBeforeLong int
}

// DefaultPomodoroConfig returns the classic 25m/5m/15m/4 rotation.
func DefaultPomodoroConfig() PomodoroConfig {
	return PomodoroConfig{
		Work:             DefaultPomodoroWork,
		ShortBreak:       DefaultPomodoroShortBreak,
		LongBreak:        DefaultPomodoroLongBreak,
		BlocksBeforeLong: DefaultBlocksBeforeLong,
	}
}

// sanitize fills non-positive fields with the classic defaults.
func (c PomodoroConfig) sanitize() PomodoroConfig {
	if c.Work <= 0 {
		c.Work = DefaultPomodoroWork
	}
	if c.ShortBreak <= 0 {
		c.ShortBreak = DefaultPomodoroShortBreak
	}
	if c.LongBreak <= 0 {
		c.LongBreak = DefaultPomodoroLongBreak
	}
	if c.BlocksBeforeLong <= 0 {
		c.BlocksBeforeLong = DefaultBlocksBeforeLong
	}
	return c
}

// PomodoroCycle tracks the rotation state: current phase, completed work
// blocks in the set, and when the current phase started (clock-driven).
type PomodoroCycle struct {
	cfg           PomodoroConfig
	clock         Clock
	phase         PomodoroPhase
	phaseStarted  time.Time
	completedWork int
}

// NewPomodoroCycle starts a cycle at the first work block. A nil clock
// selects the wall clock, matching Session.
func NewPomodoroCycle(cfg PomodoroConfig, clock Clock) *PomodoroCycle {
	if clock == nil {
		clock = wallClock{}
	}
	cfg = cfg.sanitize()
	return &PomodoroCycle{
		cfg:          cfg,
		clock:        clock,
		phase:        PomodoroWork,
		phaseStarted: clock.Now(),
	}
}

// Phase reports the current rotation phase.
func (c *PomodoroCycle) Phase() PomodoroPhase { return c.phase }

// PhaseDuration returns the configured length of the current phase.
func (c *PomodoroCycle) PhaseDuration() time.Duration {
	switch c.phase {
	case PomodoroShortBreak:
		return c.cfg.ShortBreak
	case PomodoroLongBreak:
		return c.cfg.LongBreak
	default:
		return c.cfg.Work
	}
}

// Elapsed returns wall time since the current phase started (clamped >= 0).
func (c *PomodoroCycle) Elapsed(now time.Time) time.Duration {
	if e := now.Sub(c.phaseStarted); e > 0 {
		return e
	}
	return 0
}

// Remaining returns PhaseDuration − Elapsed; the phase ends at Remaining <= 0.
func (c *PomodoroCycle) Remaining(now time.Time) time.Duration {
	return c.PhaseDuration() - c.Elapsed(now)
}

// WorkCompleted counts finished work blocks in the current set (resets after
// each long break).
func (c *PomodoroCycle) WorkCompleted() int { return c.completedWork }

// BlocksPerSet is how many work blocks form one set (long break cadence).
func (c *PomodoroCycle) BlocksPerSet() int { return c.cfg.BlocksBeforeLong }

// CurrentBlock is the current (work) or upcoming (break) work-block number,
// always in [1, BlocksPerSet].
func (c *PomodoroCycle) CurrentBlock() int {
	switch c.phase {
	case PomodoroLongBreak:
		return 1
	case PomodoroShortBreak:
		return c.completedWork + 1
	default:
		return c.completedWork + 1
	}
}

// PhaseLabel is the timer-UI label: "Work n of N" for work, "Short break" /
// "Long break" for breaks.
func (c *PomodoroCycle) PhaseLabel() string {
	if c.phase == PomodoroWork {
		return fmt.Sprintf("Work %d of %d", c.CurrentBlock(), c.cfg.BlocksBeforeLong)
	}
	if c.phase == PomodoroLongBreak {
		return "Long break"
	}
	return "Short break"
}

// Advance moves to the next phase and restarts the phase clock: finishing a
// work block enters the short break, or the long break when the set is
// complete; finishing a break enters the next work block (resetting the
// set counter after a long break).
func (c *PomodoroCycle) Advance() {
	switch c.phase {
	case PomodoroWork:
		c.completedWork++
		if c.completedWork%c.cfg.BlocksBeforeLong == 0 {
			c.phase = PomodoroLongBreak
		} else {
			c.phase = PomodoroShortBreak
		}
	default: // any break → next work block
		if c.phase == PomodoroLongBreak {
			c.completedWork = 0
		}
		c.phase = PomodoroWork
	}
	c.phaseStarted = c.clock.Now()
}
