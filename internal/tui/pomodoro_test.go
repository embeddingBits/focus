package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newPhaseModel(task string, planned time.Duration, phaseLabel string, isBreak bool) (timerModel, *fakeClock) {
	m := newTimerModel(TimerRequest{
		Task:       task,
		Planned:    planned,
		PhaseLabel: phaseLabel,
		Break:      isBreak,
	})
	fc := &fakeClock{now: time.Now()}
	m.now = func() time.Time { return fc.now }
	m.startTime = fc.now
	return m, fc
}

func TestPhaseLabelShownInTimerView(t *testing.T) {
	m, _ := newPhaseModel("write report", 25*time.Minute, "Work 1 of 4", false)
	view := m.View()
	if !strings.Contains(view, "Work 1 of 4") {
		t.Fatalf("timer view should show phase label:\n%s", view)
	}
	if !strings.Contains(view, "write report") {
		t.Fatalf("timer view should keep the task title:\n%s", view)
	}
}

func TestPlainTimerViewHasNoPhaseLabelLine(t *testing.T) {
	m := newTimerModel(TimerRequest{Task: "write report", Planned: 25 * time.Minute})
	if strings.Contains(m.View(), "Work 1 of 4") {
		t.Fatal("plain timer must not invent a phase label")
	}
}

func TestBreakSkipWithS(t *testing.T) {
	m, fc := newPhaseModel("write report", 5*time.Minute, "Short break", true)
	fc.advance(2 * time.Minute)
	m, cmd := step(m, keyRune("s"))
	if !m.done {
		t.Fatal("s during break should skip immediately (no prompts)")
	}
	if m.view == viewPrompt {
		t.Fatal("break skip must not enter the prompt view")
	}
	if cmd == nil {
		t.Fatal("skip should return tea.Quit command")
	}
	if !m.result.Completed {
		t.Fatal("skipped break must report Completed=true (break over, keep rotating)")
	}
	if m.result.ElapsedTotal != 2*time.Minute {
		t.Fatalf("skip ElapsedTotal = %v, want 2m", m.result.ElapsedTotal)
	}
}

func TestBreakExpiryAutoCompletes(t *testing.T) {
	hook := func() time.Duration { return 6 * time.Minute }
	m := newTimerModel(TimerRequest{
		Task: "write report", Planned: 5 * time.Minute,
		Elapsed: hook, PhaseLabel: "Short break", Break: true,
	})
	m, _ = step(m, tickMsg(time.Now()))
	if !m.done {
		t.Fatal("tick past break expiry should complete the break")
	}
	if m.view == viewPrompt {
		t.Fatal("expired break must not enter the prompt view")
	}
	if !m.result.Completed {
		t.Fatal("expired break must report Completed=true")
	}
}

func TestBreakDoubleQQuits(t *testing.T) {
	m, _ := newPhaseModel("write report", 5*time.Minute, "Short break", true)
	m, _ = step(m, keyRune("q"))
	if m.done {
		t.Fatal("single q must not quit the break")
	}
	m, _ = step(m, keyRune("q"))
	if !m.done {
		t.Fatal("double q should quit the break")
	}
	if m.result.Completed {
		t.Fatal("quit break must report Completed=false (stop the rotation)")
	}
}

func TestBreakFooterOffersSkip(t *testing.T) {
	m, _ := newPhaseModel("write report", 5*time.Minute, "Short break", true)
	view := m.View()
	for _, want := range []string{"Short break", "s", "skip"} {
		if !strings.Contains(view, want) {
			t.Fatalf("break view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "finish") {
		t.Fatalf("break view must not offer finish prompts:\n%s", view)
	}
}

func TestWorkExpiryStillPromptsWithPhaseLabel(t *testing.T) {
	hook := func() time.Duration { return 26 * time.Minute }
	m := newTimerModel(TimerRequest{
		Task: "write report", Planned: 25 * time.Minute,
		Elapsed: hook, PhaseLabel: "Work 1 of 4",
	})
	m, _ = step(m, tickMsg(time.Now()))
	if m.view != viewPrompt {
		t.Fatal("expired work block must still enter the prompt view")
	}
	if m.done || m.result.Completed {
		t.Fatal("prompt entry must not finish by itself")
	}
}

func TestBreakViewGolden(t *testing.T) {
	m, _ := newPhaseModel("write report", 5*time.Minute, "Short break", true)
	got := m.View()
	if !strings.Contains(got, "write report") ||
		!strings.Contains(got, "Short break") ||
		!strings.Contains(got, "00:05:00") ||
		!strings.Contains(got, "Running") ||
		!strings.Contains(got, "skip") {
		t.Fatalf("break golden view missing pieces:\n%s", got)
	}
}

// TestBreakTimerKeyRouting ensures prompt-only keys never leak into the
// break countdown view (breaks have no prompt view at all).
func TestBreakPauseStillWorks(t *testing.T) {
	m, fc := newPhaseModel("write report", 5*time.Minute, "Short break", true)
	fc.advance(time.Minute)
	m, _ = step(m, keyRune("p"))
	if !m.paused {
		t.Fatal("p should pause a break too")
	}
	fc.advance(time.Minute) // paused wall time must not count
	if got := m.elapsed(); got != time.Minute {
		t.Fatalf("break elapsed while paused = %v, want 1m", got)
	}
	var _ tea.Msg = keyRune("p")
}
