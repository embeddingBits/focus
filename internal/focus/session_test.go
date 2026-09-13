package focus

import (
	"errors"
	"testing"
	"time"
)

// fakeClock is a manual clock for deterministic tests (no sleeps, no I/O).
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestSession() (*Session, *fakeClock) {
	fc := &fakeClock{now: time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)}
	return New(fc), fc
}

func TestStartDefaultsNonPositivePlanned(t *testing.T) {
	for _, planned := range []time.Duration{0, -5 * time.Minute} {
		s, _ := newTestSession()
		if err := s.Start("task", planned); err != nil {
			t.Fatalf("Start: %v", err)
		}
		if s.State() != StateRunning {
			t.Fatalf("state = %q, want running", s.State())
		}
		if s.Planned != DefaultLength {
			t.Fatalf("Planned = %v, want %v", s.Planned, DefaultLength)
		}
	}
}

func TestStartTwiceErrors(t *testing.T) {
	s, _ := newTestSession()
	if err := s.Start("a", time.Minute); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	var te ErrTransition
	if err := s.Start("b", time.Minute); !errors.As(err, &te) {
		t.Fatalf("second Start err = %v, want ErrTransition", err)
	}
}

func TestPauseResumeAccumulates(t *testing.T) {
	s, fc := newTestSession()
	if err := s.Start("task", time.Hour); err != nil {
		t.Fatal(err)
	}
	fc.advance(10 * time.Minute)
	if err := s.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	fc.advance(2 * time.Minute)
	if got := s.Elapsed(fc.Now()); got != 10*time.Minute {
		t.Fatalf("elapsed while paused = %v, want 10m", got)
	}
	if err := s.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if s.PausedTotal != 2*time.Minute {
		t.Fatalf("PausedTotal = %v, want 2m", s.PausedTotal)
	}
	if got := s.Elapsed(fc.Now()); got != 10*time.Minute {
		t.Fatalf("elapsed after resume = %v, want 10m", got)
	}
	fc.advance(5 * time.Minute)
	if got := s.Elapsed(fc.Now()); got != 15*time.Minute {
		t.Fatalf("elapsed after resume+5m = %v, want 15m", got)
	}
}

func TestDoublePauseAndResumeWithoutPauseError(t *testing.T) {
	s, _ := newTestSession()
	var te ErrTransition
	if err := s.Pause(); !errors.As(err, &te) {
		t.Fatalf("pause from idle err = %v, want ErrTransition", err)
	}
	if err := s.Resume(); !errors.As(err, &te) {
		t.Fatalf("resume without pause err = %v, want ErrTransition", err)
	}
	if err := s.Start("task", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := s.Pause(); !errors.As(err, &te) {
		t.Fatalf("double pause err = %v, want ErrTransition", err)
	}
}

func TestFinishFromRunningAndPausedFreezesElapsed(t *testing.T) {
	for _, fromPaused := range []bool{false, true} {
		s, fc := newTestSession()
		if err := s.Start("task", time.Hour); err != nil {
			t.Fatal(err)
		}
		fc.advance(20 * time.Minute)
		if fromPaused {
			if err := s.Pause(); err != nil {
				t.Fatal(err)
			}
			fc.advance(3 * time.Minute) // paused wall time must not count
		}
		if err := s.Finish("did it", "next"); err != nil {
			t.Fatalf("Finish: %v", err)
		}
		if s.State() != StateDone {
			t.Fatalf("state = %q, want done", s.State())
		}
		if got := s.Elapsed(fc.Now()); got != 20*time.Minute {
			t.Fatalf("fromPaused=%v elapsed = %v, want 20m", fromPaused, got)
		}
		fc.advance(time.Hour)
		if got := s.Elapsed(fc.Now()); got != 20*time.Minute {
			t.Fatalf("fromPaused=%v elapsed after wait = %v, want frozen 20m", fromPaused, got)
		}
		var te ErrTransition
		if err := s.Finish("x", "y"); !errors.As(err, &te) {
			t.Fatalf("second Finish err = %v, want ErrTransition", err)
		}
	}
}

func TestFinishFromIdleErrors(t *testing.T) {
	s, _ := newTestSession()
	var te ErrTransition
	if err := s.Finish("x", "y"); !errors.As(err, &te) {
		t.Fatalf("finish from idle err = %v, want ErrTransition", err)
	}
}

func TestAbandon(t *testing.T) {
	s, fc := newTestSession()
	if err := s.Start("task", time.Hour); err != nil {
		t.Fatal(err)
	}
	fc.advance(5 * time.Minute)
	if err := s.Abandon(); err != nil {
		t.Fatalf("Abandon: %v", err)
	}
	if s.State() != StateAbandoned {
		t.Fatalf("state = %q, want abandoned", s.State())
	}
	if s.Completed {
		t.Fatal("Completed = true after abandon, want false")
	}
}

func TestProgressAndRemaining(t *testing.T) {
	s, fc := newTestSession()
	if err := s.Start("task", 25*time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.advance(5 * time.Minute)
	if got := s.Progress(fc.Now()); got < 0.19 || got > 0.21 {
		t.Fatalf("progress = %v, want ~0.2", got)
	}
	if got := s.Remaining(fc.Now()); got != 20*time.Minute {
		t.Fatalf("remaining = %v, want 20m", got)
	}
	fc.advance(20 * time.Minute)
	if got := s.Remaining(fc.Now()); got > 0 {
		t.Fatalf("remaining at boundary = %v, want <= 0", got)
	}
	if got := s.Progress(fc.Now()); got != 1 {
		t.Fatalf("progress past end = %v, want clamped 1", got)
	}
}
