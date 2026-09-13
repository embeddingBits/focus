package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- harness helpers (no TTY needed) ---

func keyRune(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyType(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func step(m timerModel, msg tea.Msg) (timerModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	tm, ok := next.(timerModel)
	if !ok {
		panic("tui test: Update returned non-timerModel")
	}
	return tm, cmd
}

// fakeClock lets tests advance wall time deterministically.
type fakeClock struct{ now time.Time }

func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestModel(planned time.Duration, hook func() time.Duration) (timerModel, *fakeClock) {
	m := newTimerModel(TimerRequest{Task: "write report", Planned: planned, Elapsed: hook})
	fc := &fakeClock{now: time.Now()}
	m.now = func() time.Time { return fc.now }
	m.startTime = fc.now
	return m, fc
}

func TestDefaultPlanned(t *testing.T) {
	m := newTimerModel(TimerRequest{Task: "x"})
	if m.planned != DefaultPlanned {
		t.Fatalf("planned = %v, want %v", m.planned, DefaultPlanned)
	}
}

func TestPauseResumeAccumulates(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)

	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	if !m.paused {
		t.Fatal("after p: want paused")
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed while paused = %v, want 10m", got)
	}

	// 2 more wall minutes pass while paused: elapsed must stay frozen.
	fc.advance(2 * time.Minute)
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed after paused wait = %v, want frozen 10m", got)
	}
	if !strings.Contains(m.View(), "Paused") {
		t.Fatal("view while paused should contain Paused")
	}

	m, _ = step(m, keyRune("p"))
	if m.paused {
		t.Fatal("after second p: want running")
	}
	if m.pausedTotal != 2*time.Minute {
		t.Fatalf("pausedTotal = %v, want 2m", m.pausedTotal)
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed after resume = %v, want 10m", got)
	}

	// 5 running minutes: elapsed advances again.
	fc.advance(5 * time.Minute)
	if got := m.elapsed(); got != 15*time.Minute {
		t.Fatalf("elapsed after resume+5m = %v, want 15m", got)
	}
}

func TestFinishFlowPromptsAndResult(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)

	m, _ = step(m, keyRune("s"))
	if m.view != viewPrompt {
		t.Fatal("after s: want prompt view")
	}

	m.accInput.SetValue("shipped the report")
	m.nextInput.SetValue("send invoice")

	// enter on first field advances to second; enter on second submits.
	m, _ = step(m, keyType(tea.KeyEnter))
	if m.focusIdx != 1 || m.done {
		t.Fatal("first enter should move to next field, not submit")
	}
	m, cmd := step(m, keyType(tea.KeyEnter))
	if !m.done {
		t.Fatal("second enter should submit")
	}
	if cmd == nil {
		t.Fatal("submit should return tea.Quit command")
	}
	r := m.result
	if !r.Completed {
		t.Fatal("result.Completed = false, want true")
	}
	if r.Accomplishment != "shipped the report" {
		t.Fatalf("Accomplishment = %q", r.Accomplishment)
	}
	if r.Next != "send invoice" {
		t.Fatalf("Next = %q", r.Next)
	}
}

func TestPromptTypingHeadless(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)
	m, _ = step(m, keyRune("s"))
	for _, k := range []string{"h", "i"} {
		m, _ = step(m, keyRune(k))
	}
	if got := m.accInput.Value(); got != "hi" {
		t.Fatalf("typed acc = %q, want %q", got, "hi")
	}
	m, _ = step(m, keyType(tea.KeyTab))
	m, _ = step(m, keyRune("!"))
	if got := m.nextInput.Value(); got != "!" {
		t.Fatalf("typed next = %q, want %q", got, "!")
	}
}

func TestAbandonRequiresDoubleQ(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)

	m, _ = step(m, keyRune("q"))
	if m.done {
		t.Fatal("single q must not abandon")
	}
	if !strings.Contains(m.View(), "Press q again") {
		t.Fatal("after first q: view should show abandon confirm")
	}

	// Any other key disarms the confirm.
	m, _ = step(m, keyRune("p"))
	if m.qArmed {
		t.Fatal("other key should disarm q confirm")
	}
	if !m.paused {
		t.Fatal("p should still pause normally after disarming")
	}
	m, _ = step(m, keyRune("p")) // resume

	m, _ = step(m, keyRune("q"))
	m, _ = step(m, keyRune("q"))
	if !m.done {
		t.Fatal("double q should abandon")
	}
	if m.result.Completed {
		t.Fatal("abandoned result must have Completed=false")
	}
	if m.result.Accomplishment != "" || m.result.Next != "" {
		t.Fatal("abandoned result must carry no prompts")
	}
}

func TestCtrlCAbandons(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)
	m, _ = step(m, keyType(tea.KeyCtrlC))
	if !m.done || m.result.Completed {
		t.Fatal("ctrl+c should abandon immediately")
	}
}

func TestExpiryAutoPrompts(t *testing.T) {
	hook := func() time.Duration { return 26 * time.Minute }
	m, _ := newTestModel(25*time.Minute, hook)
	if m.remaining() > 0 {
		t.Fatal("fixture should be past expiry")
	}
	m, _ = step(m, tickMsg(time.Now()))
	if m.view != viewPrompt {
		t.Fatal("tick past expiry should auto-enter prompt view")
	}
}

func TestClocksAndProgress(t *testing.T) {
	half := 12*time.Minute + 30*time.Second
	hook := func() time.Duration { return half }
	m, _ := newTestModel(25*time.Minute, hook)

	if got := m.elapsed(); got != half {
		t.Fatalf("elapsed = %v, want %v", half, got)
	}
	if got := m.remaining(); got != half {
		t.Fatalf("remaining = %v, want %v", half, got)
	}
	if p := m.progressPct(); p != 0.5 {
		t.Fatalf("progress = %v, want 0.5", p)
	}
	view := m.View()
	for _, want := range []string{"write report", "00:12:30", "Running", "p", "s", "q"} {
		if !strings.Contains(view, want) {
			t.Fatalf("timer view missing %q\n%s", want, view)
		}
	}
}

func TestWindowSizeClampsBar(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)

	m, _ = step(m, tea.WindowSizeMsg{Width: 200, Height: 50})
	if m.progress.Width != maxBarWidth {
		t.Fatalf("wide width: bar = %d, want %d", m.progress.Width, maxBarWidth)
	}
	m, _ = step(m, tea.WindowSizeMsg{Width: 10, Height: 20})
	if m.progress.Width != minBarWidth {
		t.Fatalf("narrow width: bar = %d, want %d", m.progress.Width, minBarWidth)
	}
	m, _ = step(m, tea.WindowSizeMsg{Width: 60, Height: 20})
	if m.progress.Width != 56 {
		t.Fatalf("normal width: bar = %d, want 56", m.progress.Width)
	}
}

func TestEscBackToTimer(t *testing.T) {
	m, _ := newTestModel(25*time.Minute, nil)
	m, _ = step(m, keyRune("s"))
	if m.view != viewPrompt {
		t.Fatal("setup: want prompt view")
	}
	m, _ = step(m, keyType(tea.KeyEsc))
	if m.view != viewTimer || m.done {
		t.Fatal("esc should return to timer without finishing")
	}
}

func TestFinishFromPausedFoldsPause(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	fc.advance(3 * time.Minute)
	m, _ = step(m, keyRune("s")) // finish from paused: settle at s-press
	if m.paused {
		t.Fatal("entering prompt should settle pending pause")
	}
	fc.advance(time.Minute) // typing prompts is not focus time
	m, _ = step(m, keyType(tea.KeyEnter))
	m, _ = step(m, keyType(tea.KeyEnter))
	if !m.done || !m.result.Completed {
		t.Fatal("want finished result")
	}
	if m.result.PausedTotal != 3*time.Minute {
		t.Fatalf("finish PausedTotal = %v, want 3m", m.result.PausedTotal)
	}
	if m.result.ElapsedTotal != 11*time.Minute {
		t.Fatalf("finish ElapsedTotal = %v, want 11m", m.result.ElapsedTotal)
	}
}

func TestAbandonKeepsPauseAccounting(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	fc.advance(3 * time.Minute)
	m, _ = step(m, keyRune("q"))
	m, _ = step(m, keyRune("q"))
	if m.result.PausedTotal != 3*time.Minute {
		t.Fatalf("abandon PausedTotal = %v, want 3m", m.result.PausedTotal)
	}
	if m.result.ElapsedTotal != 10*time.Minute {
		t.Fatalf("abandon ElapsedTotal = %v, want 10m", m.result.ElapsedTotal)
	}
}
