package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- break mode tests (TDD; timer-model transitions via step/fakeClock) ---

// breakHook captures persisted break callbacks for assertions.
type breakHook struct {
	calls []BreakInfo
}

func (h *breakHook) fn() func(BreakInfo) {
	return func(b BreakInfo) { h.calls = append(h.calls, b) }
}

// enterBreakFromWork advances 10m of work, then presses b.
func enterBreakFromWork(t *testing.T, planned time.Duration) (timerModel, *fakeClock, *breakHook) {
	t.Helper()
	m, fc := newTestModel(planned, nil)
	fc.advance(10 * time.Minute)
	hook := &breakHook{}
	m.onBreak = hook.fn()
	m, _ = step(m, keyRune("b"))
	if m.view != viewBreak {
		t.Fatalf("after b: view = %v, want viewBreak", m.view)
	}
	return m, fc, hook
}

func TestBreakEntrySnapshotsFiveMinutesMMSelectedAndPauses(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b"))

	if m.view != viewBreak {
		t.Fatalf("after b: view = %v, want viewBreak", m.view)
	}
	if m.breakRemain != 5*time.Minute {
		t.Fatalf("breakRemain = %v, want 5m", m.breakRemain)
	}
	if m.breakPlanned != 5*time.Minute {
		t.Fatalf("breakPlanned = %v, want 5m", m.breakPlanned)
	}
	if m.breakSel != breakFieldMM {
		t.Fatalf("breakSel = %v, want MM selected", m.breakSel)
	}
	if !m.paused {
		t.Fatal("entering break should auto-pause the work session")
	}
}

func TestBreakEntryFreezesWorkElapsed(t *testing.T) {
	m, fc, _ := enterBreakFromWork(t, time.Hour)
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed at break entry = %v, want frozen 10m", got)
	}
	fc.advance(2 * time.Minute)
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed during break = %v, want frozen 10m", got)
	}
}

func TestBreakEntryClearsQArm(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("q")) // arm abandon confirm
	m, _ = step(m, keyRune("b"))
	if m.qArmed {
		t.Fatal("entering break must clear a pending q-arm")
	}
	m, _ = step(m, keyType(tea.KeyEsc))
	if m.qArmed {
		t.Fatal("leaving break must not restore a q-arm")
	}
	m, _ = step(m, keyRune("q"))
	if m.done {
		t.Fatal("single q after break must only arm, not abandon")
	}
}

func TestBreakStepsAndFieldMoves(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b")) // 00:05:00, MM selected

	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != 6*time.Minute {
		t.Fatalf("MM up from 5m = %v, want 6m", m.breakRemain)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if m.breakRemain != 5*time.Minute {
		t.Fatalf("MM down from 6m = %v, want 5m", m.breakRemain)
	}

	m, _ = step(m, keyType(tea.KeyLeft))
	if m.breakSel != breakFieldHH {
		t.Fatal("left from MM should select HH")
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != time.Hour+5*time.Minute {
		t.Fatalf("HH up from 00:05 = %v, want 01:05", m.breakRemain)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if m.breakRemain != 5*time.Minute {
		t.Fatalf("HH down from 01:05 = %v, want 00:05", m.breakRemain)
	}

	// Left stops at HH; right stops at MM (Qt semantics, no wrap).
	m, _ = step(m, keyType(tea.KeyLeft))
	if m.breakSel != breakFieldHH {
		t.Fatal("left at HH should stay on HH")
	}
	m, _ = step(m, keyType(tea.KeyRight))
	if m.breakSel != breakFieldMM {
		t.Fatal("right from HH should select MM")
	}
	m, _ = step(m, keyType(tea.KeyRight))
	if m.breakSel != breakFieldMM {
		t.Fatal("right at MM should stay on MM")
	}
}

func TestBreakArrowStringSpellingAlsoSteps(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b"))

	// msg.String() "up" via runes must step like tea.KeyUp (both spellings).
	m, _ = step(m, keyRune("up"))
	if m.breakRemain != 6*time.Minute {
		t.Fatalf("rune-spelled up = %v, want 6m", m.breakRemain)
	}
	m, _ = step(m, keyRune("down"))
	if m.breakRemain != 5*time.Minute {
		t.Fatalf("rune-spelled down = %v, want 5m", m.breakRemain)
	}
	m, _ = step(m, keyRune("left"))
	if m.breakSel != breakFieldHH {
		t.Fatal("rune-spelled left should select HH")
	}
	m, _ = step(m, keyRune("right"))
	if m.breakSel != breakFieldMM {
		t.Fatal("rune-spelled right should select MM")
	}
}

func TestBreakClampDownAtZeroStays(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b"))
	for range 5 {
		m, _ = step(m, keyType(tea.KeyDown)) // MM 5 → 0
	}
	if m.breakRemain != 0 {
		t.Fatalf("down to zero = %v, want 0", m.breakRemain)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if m.breakRemain != 0 {
		t.Fatalf("down at 00:00 = %v, want stay at 0", m.breakRemain)
	}
	m, _ = step(m, keyType(tea.KeyLeft))
	m, _ = step(m, keyType(tea.KeyDown))
	if m.breakRemain != 0 {
		t.Fatalf("HH down at 00:00 = %v, want stay at 0", m.breakRemain)
	}
}

func TestBreakClampUpCapsNoCarryNoWrap(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b"))

	// MM saturates at 59 with no carry into hours.
	m.breakRemain = time.Hour + 59*time.Minute
	m.breakPlanned = m.breakRemain
	m.breakSel = breakFieldMM
	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != time.Hour+59*time.Minute {
		t.Fatalf("MM up at x:59 = %v, want no carry, stay", m.breakRemain)
	}

	// HH saturates at 23.
	m.breakRemain = 23*time.Hour + 30*time.Minute
	m.breakPlanned = m.breakRemain
	m.breakSel = breakFieldHH
	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != 23*time.Hour+30*time.Minute {
		t.Fatalf("HH up at 23:30 = %v, want stay", m.breakRemain)
	}

	// Total caps at 23:59 via MM stepping at the top hour.
	m.breakRemain = 23*time.Hour + 58*time.Minute
	m.breakPlanned = m.breakRemain
	m.breakSel = breakFieldMM
	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != 23*time.Hour+59*time.Minute {
		t.Fatalf("MM up at 23:58 = %v, want 23:59", m.breakRemain)
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if m.breakRemain != 23*time.Hour+59*time.Minute {
		t.Fatalf("MM up at 23:59 = %v, want cap stay", m.breakRemain)
	}
}

func TestBreakStepToZeroPinsSeconds(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m, _ = step(m, keyRune("b"))
	m.breakRemain = 90 * time.Second // 00:01:30 mid-countdown value
	m.breakSel = breakFieldMM
	m, _ = step(m, keyType(tea.KeyDown))
	if m.breakRemain != 0 {
		t.Fatalf("step to 00:00 = %v, want pinned 0 (no stale seconds)", m.breakRemain)
	}
}

func TestBreakTickDecrementsWallDelta(t *testing.T) {
	m, fc, _ := enterBreakFromWork(t, time.Hour)
	fc.advance(70 * time.Second)
	m, _ = step(m, tickMsg(fc.now))
	if m.breakRemain != 5*time.Minute-70*time.Second {
		t.Fatalf("remain after 70s = %v, want 3m50s", m.breakRemain)
	}
	if m.view != viewBreak {
		t.Fatal("break should still be active after 70s of a 5m break")
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("work elapsed during break tick = %v, want frozen 10m", got)
	}
}

func TestBreakExpiryEndsResumesAndPersists(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(5*time.Minute + time.Second)
	m, _ = step(m, tickMsg(fc.now))

	if m.view != viewTimer {
		t.Fatal("expiry should return to the timer view")
	}
	if m.paused {
		t.Fatal("expiry should resume the work session")
	}
	if m.pausedTotal != 5*time.Minute+time.Second {
		t.Fatalf("pausedTotal = %v, want break interval folded in", m.pausedTotal)
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("work elapsed after break = %v, want 10m", got)
	}
	if len(hook.calls) != 1 {
		t.Fatalf("expiry should persist one break row, got %d", len(hook.calls))
	}
	got := hook.calls[0]
	if got.Planned != 5*time.Minute {
		t.Fatalf("break Planned = %v, want 5m", got.Planned)
	}
	if got.Taken != 5*time.Minute+time.Second {
		t.Fatalf("break Taken = %v, want wall interval", got.Taken)
	}
	if got.EndedAt.Sub(got.StartedAt) != got.Taken {
		t.Fatal("break StartedAt/EndedAt should span Taken")
	}
}

func TestBreakEnterEndsEarlySamePath(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(time.Minute)
	m, _ = step(m, tickMsg(fc.now)) // remain 4:00
	m, _ = step(m, keyType(tea.KeyEnter))

	if m.view != viewTimer || m.paused {
		t.Fatal("enter should end break and resume work")
	}
	if len(hook.calls) != 1 {
		t.Fatalf("early enter should persist, got %d calls", len(hook.calls))
	}
	if hook.calls[0].Taken != time.Minute {
		t.Fatalf("early Taken = %v, want 1m", hook.calls[0].Taken)
	}
	if hook.calls[0].Planned != 5*time.Minute {
		t.Fatalf("early Planned = %v, want chosen 5m", hook.calls[0].Planned)
	}
	if m.pausedTotal != time.Minute {
		t.Fatalf("pausedTotal = %v, want 1m folded", m.pausedTotal)
	}
}

func TestBreakEnterOnZeroCancels(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(30 * time.Second)
	for range 5 {
		m, _ = step(m, keyType(tea.KeyDown)) // MM 5 → 0, remain pinned 0
	}
	m, _ = step(m, keyType(tea.KeyEnter))

	if m.view != viewTimer || m.paused {
		t.Fatal("enter on 00:00 should cancel back to timer, resumed")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("enter on 00:00 must persist nothing, got %d calls", len(hook.calls))
	}
	if m.pausedTotal != 30*time.Second {
		t.Fatalf("pausedTotal = %v, want break wall time folded", m.pausedTotal)
	}
}

func TestBreakSEndsEarly(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(45 * time.Second)
	m, _ = step(m, tickMsg(fc.now))
	m, _ = step(m, keyRune("s"))

	if m.view != viewTimer || m.paused {
		t.Fatal("s should end break early and resume work")
	}
	if len(hook.calls) != 1 || hook.calls[0].Taken != 45*time.Second {
		t.Fatalf("s should persist Taken=45s, got %+v", hook.calls)
	}
}

func TestBreakSOnZeroPersistsNothing(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(10 * time.Second)
	for range 5 {
		m, _ = step(m, keyType(tea.KeyDown))
	}
	m, _ = step(m, keyRune("s"))
	if m.view != viewTimer || m.paused {
		t.Fatal("s on 00:00 should cancel back to timer, resumed")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("s on 00:00 must persist nothing, got %d calls", len(hook.calls))
	}
}

func TestBreakEscCancelsResumesPersistsNothing(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(45 * time.Second)
	m, _ = step(m, tickMsg(fc.now))
	m, _ = step(m, keyType(tea.KeyEsc))

	if m.view != viewTimer || m.paused {
		t.Fatal("esc should cancel break and resume work")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("esc must persist nothing, got %d calls", len(hook.calls))
	}
	if m.pausedTotal != 45*time.Second {
		t.Fatalf("pausedTotal = %v, want break wall time folded", m.pausedTotal)
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("work elapsed after cancel = %v, want 10m", got)
	}
}

func TestBreakQCancelsLikeEsc(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	fc.advance(20 * time.Second)
	m, _ = step(m, keyRune("q"))
	if m.view != viewTimer || m.paused {
		t.Fatal("q in break should cancel like esc")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("q-cancel must persist nothing, got %d calls", len(hook.calls))
	}
}

func TestBreakBDoesNothing(t *testing.T) {
	m, fc, _ := enterBreakFromWork(t, time.Hour)
	fc.advance(10 * time.Second)
	m, _ = step(m, tickMsg(fc.now))
	before := m.breakRemain
	m, _ = step(m, keyRune("b"))
	if m.breakRemain != before || m.breakSel != breakFieldMM || m.view != viewBreak {
		t.Fatal("b during break must not re-snapshot or leave the view")
	}
}

func TestBreakPIsNoop(t *testing.T) {
	m, fc, _ := enterBreakFromWork(t, time.Hour)
	fc.advance(10 * time.Second)
	m, _ = step(m, tickMsg(fc.now))
	before := m.breakRemain
	m, _ = step(m, keyRune("p"))
	if m.view != viewBreak || !m.paused || m.breakRemain != before {
		t.Fatal("p during break must leave break state untouched")
	}
}

func TestBreakCtrlCAbandons(t *testing.T) {
	m, _, hook := enterBreakFromWork(t, time.Hour)
	m, _ = step(m, keyType(tea.KeyCtrlC))
	if !m.done || m.result.Completed {
		t.Fatal("ctrl+c during break should abandon the session")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("abandon must persist no break row, got %d calls", len(hook.calls))
	}
}

func TestBreakAdjustPersistsChosenLength(t *testing.T) {
	m, fc, hook := enterBreakFromWork(t, time.Hour)
	m, _ = step(m, keyType(tea.KeyUp)) // MM 5 → 6, chosen 6:00
	fc.advance(time.Minute)
	m, _ = step(m, tickMsg(fc.now))
	m, _ = step(m, keyType(tea.KeyEnter))
	if len(hook.calls) != 1 {
		t.Fatalf("want 1 break row, got %d", len(hook.calls))
	}
	if hook.calls[0].Planned != 6*time.Minute {
		t.Fatalf("Planned = %v, want adjusted 6m", hook.calls[0].Planned)
	}
	if hook.calls[0].Taken != time.Minute {
		t.Fatalf("Taken = %v, want 1m", hook.calls[0].Taken)
	}
}

func TestBreakDefaultOverride(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	m.breakDefault = 90 * time.Second
	m, _ = step(m, keyRune("b"))
	if m.breakRemain != 90*time.Second {
		t.Fatalf("breakRemain = %v, want env-shortened 90s", m.breakRemain)
	}
}

func TestBreakViewDocumentsKeys(t *testing.T) {
	m, _, _ := enterBreakFromWork(t, time.Hour)
	view := m.View()
	for _, want := range []string{"Break", "00:05:00", "up/down", "left/right", "enter", "esc", "session paused"} {
		if !strings.Contains(view, want) {
			t.Fatalf("break view missing %q\n%s", want, view)
		}
	}
}

func TestTimerViewHintsBreakKey(t *testing.T) {
	m, _ := newTestModel(time.Hour, nil)
	if !strings.Contains(m.View(), "b") {
		t.Fatalf("timer footer should hint the b key:\n%s", m.View())
	}
}
