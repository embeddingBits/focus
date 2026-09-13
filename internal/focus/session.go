// Package focus holds the session state machine and the persistence port.
//
// The engine owns pause math authoritatively (frozen plan §5); the TUI keeps
// its own display accumulator using the same rules and returns it for
// persist-time use. This package imports only the standard library.
package focus

import (
	"fmt"
	"time"
)

// DefaultLength is the planned session length used when a non-positive
// planned duration is given (frozen plan §5: 25 minutes).
const DefaultLength = 25 * time.Minute

// State is the session lifecycle state.
type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateDone      State = "done"
	StateAbandoned State = "abandoned"
)

// Clock abstracts time for testability; production uses the wall clock.
type Clock interface{ Now() time.Time }

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

// ErrTransition reports an illegal state-machine transition. It never panics;
// callers receive this as a typed error.
type ErrTransition struct {
	From State
	Op   string
}

func (e ErrTransition) Error() string {
	return fmt.Sprintf("focus: cannot %s from state %q", e.Op, e.From)
}

// Session is the in-memory session engine: pure, no I/O, clock-injected.
type Session struct {
	ID             int64
	Task           string
	Planned        time.Duration
	StartedAt      time.Time
	EndedAt        *time.Time
	PausedTotal    time.Duration
	pausedAt       *time.Time
	Accomplishment string
	Next           string
	Completed      bool
	state          State
	clock          Clock
}

// New returns an idle Session using clock (nil selects the wall clock).
func New(clock Clock) *Session {
	if clock == nil {
		clock = wallClock{}
	}
	return &Session{state: StateIdle, clock: clock}
}

// State reports the current lifecycle state.
func (s *Session) State() State { return s.state }

// Start moves idle → running. A non-positive planned defaults to
// DefaultLength. Records startedAt from the session clock.
func (s *Session) Start(task string, planned time.Duration) error {
	if s.state != StateIdle {
		return ErrTransition{From: s.state, Op: "start"}
	}
	if planned <= 0 {
		planned = DefaultLength
	}
	s.Task = task
	s.Planned = planned
	s.StartedAt = s.clock.Now()
	s.state = StateRunning
	return nil
}

// Pause moves running → paused, recording when the pause began.
func (s *Session) Pause() error {
	if s.state != StateRunning {
		return ErrTransition{From: s.state, Op: "pause"}
	}
	now := s.clock.Now()
	s.pausedAt = &now
	s.state = StatePaused
	return nil
}

// Resume moves paused → running, folding the paused interval into PausedTotal.
func (s *Session) Resume() error {
	if s.state != StatePaused {
		return ErrTransition{From: s.state, Op: "resume"}
	}
	s.PausedTotal += s.clock.Now().Sub(*s.pausedAt)
	s.pausedAt = nil
	s.state = StateRunning
	return nil
}

// settle folds a pending pause interval into PausedTotal (auto-resume
// accounting for Finish/Abandon issued while paused).
func (s *Session) settle() {
	if s.state == StatePaused && s.pausedAt != nil {
		s.PausedTotal += s.clock.Now().Sub(*s.pausedAt)
		s.pausedAt = nil
	}
}

// Finish moves running/paused → done, freezing Elapsed and recording the
// accomplishment/next prompts.
func (s *Session) Finish(accomplishment, next string) error {
	if s.state != StateRunning && s.state != StatePaused {
		return ErrTransition{From: s.state, Op: "finish"}
	}
	s.settle()
	now := s.clock.Now()
	s.EndedAt = &now
	s.Accomplishment = accomplishment
	s.Next = next
	s.Completed = true
	s.state = StateDone
	return nil
}

// Abandon moves running/paused → abandoned. The partial session persists
// (Completed=false) for honest history; stats exclude it.
func (s *Session) Abandon() error {
	if s.state != StateRunning && s.state != StatePaused {
		return ErrTransition{From: s.state, Op: "abandon"}
	}
	s.settle()
	now := s.clock.Now()
	s.EndedAt = &now
	s.Completed = false
	s.state = StateAbandoned
	return nil
}

// Elapsed returns pause-correct elapsed time:
// (now or endedAt − startedAt) − pausedTotal − (now − pausedAt if paused).
func (s *Session) Elapsed(now time.Time) time.Duration {
	if s.state == StateIdle {
		return 0
	}
	end := now
	if s.EndedAt != nil {
		end = *s.EndedAt
	}
	e := end.Sub(s.StartedAt) - s.PausedTotal
	if s.state == StatePaused && s.pausedAt != nil {
		e -= end.Sub(*s.pausedAt)
	}
	if e < 0 {
		e = 0
	}
	return e
}

// Remaining returns planned − Elapsed; the timer fires done at Remaining <= 0.
func (s *Session) Remaining(now time.Time) time.Duration {
	return s.Planned - s.Elapsed(now)
}

// Progress returns Elapsed/Planned clamped to [0,1].
func (s *Session) Progress(now time.Time) float64 {
	if s.Planned <= 0 {
		return 0
	}
	p := float64(s.Elapsed(now)) / float64(s.Planned)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}
