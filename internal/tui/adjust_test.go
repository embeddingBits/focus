package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- work-session time adjuster (TDD; timer-model transitions via step/fakeClock) ---
//
// While paused, up/down step the selected HH/MM/SS field of the remaining
// time (MM selected by default) and left/right move between the three
// fields — the same Qt section semantics as the break editor (clamp,
// no carry, no wrap). While running, arrows change nothing.

func TestWorkAdjustArrowsIgnoredWhileRunning(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	for _, k := range []tea.KeyMsg{keyType(tea.KeyUp), keyType(tea.KeyDown), keyType(tea.KeyLeft), keyType(tea.KeyRight)} {
		m, _ = step(m, k)
	}
	if m.planned != time.Hour {
		t.Fatalf("planned after running arrows = %v, want 1h", m.planned)
	}
	if got := m.remaining(); got != 50*time.Minute {
		t.Fatalf("remaining after running arrows = %v, want 50m", got)
	}
	if m.adjustSel != hmsMM {
		t.Fatal("running arrows must not move the field selection")
	}
}

func TestWorkAdjustPausedUpAddsMinute(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	if !m.paused {
		t.Fatal("setup: want paused")
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != 51*time.Minute {
		t.Fatalf("remaining after paused MM up = %v, want 51m", got)
	}
	if got := m.elapsed(); got != 10*time.Minute {
		t.Fatalf("elapsed during adjust = %v, want frozen 10m", got)
	}
	if m.planned != 61*time.Minute {
		t.Fatalf("planned after adjust = %v, want 61m (elapsed + remaining)", m.planned)
	}
}

func TestWorkAdjustPausedDownSubtractsMinute(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	m, _ = step(m, keyType(tea.KeyDown))
	if got := m.remaining(); got != 49*time.Minute {
		t.Fatalf("remaining after paused MM down = %v, want 49m", got)
	}
}

func TestWorkAdjustFieldMovesAndSeconds(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p")) // remaining 50m, MM selected

	m, _ = step(m, keyType(tea.KeyLeft))
	if m.adjustSel != hmsHH {
		t.Fatal("left from MM should select HH")
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != time.Hour+50*time.Minute {
		t.Fatalf("HH up from 00:50 = %v, want 01:50", got)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if got := m.remaining(); got != 50*time.Minute {
		t.Fatalf("HH down from 01:50 = %v, want 00:50", got)
	}

	// Right walks HH→MM→SS and stops at SS.
	m, _ = step(m, keyType(tea.KeyRight))
	m, _ = step(m, keyType(tea.KeyRight))
	if m.adjustSel != hmsSS {
		t.Fatal("right from MM should select SS")
	}
	m, _ = step(m, keyType(tea.KeyRight))
	if m.adjustSel != hmsSS {
		t.Fatal("right at SS should stay on SS")
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != 50*time.Minute+time.Second {
		t.Fatalf("SS up from 00:50:00 = %v, want 00:50:01", got)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if got := m.remaining(); got != 50*time.Minute {
		t.Fatalf("SS down from 00:50:01 = %v, want 00:50:00", got)
	}

	m, _ = step(m, keyType(tea.KeyLeft))
	if m.adjustSel != hmsMM {
		t.Fatal("left from SS should select MM")
	}
	m, _ = step(m, keyType(tea.KeyLeft))
	m, _ = step(m, keyType(tea.KeyLeft))
	if m.adjustSel != hmsHH {
		t.Fatal("left at HH should stay on HH")
	}
}

func TestWorkAdjustClampsNoCarryNoWrap(t *testing.T) {
	m, fc := newTestModel(24*time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p")) // remaining 23:50:00

	// HH saturates at 23 with no wrap.
	m.adjustSel = hmsHH
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != 23*time.Hour+50*time.Minute {
		t.Fatalf("HH up at 23:50 = %v, want stay", got)
	}

	// MM saturates at 59 with no carry into hours.
	m.planned = 10*time.Minute + time.Hour + 59*time.Minute
	m.adjustSel = hmsMM
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != time.Hour+59*time.Minute {
		t.Fatalf("MM up at 1:59 = %v, want no carry, stay", got)
	}

	// SS saturates at 59 with no carry into minutes.
	m.planned = 10*time.Minute + 59*time.Second
	m.adjustSel = hmsSS
	m, _ = step(m, keyType(tea.KeyUp))
	if got := m.remaining(); got != 59*time.Second {
		t.Fatalf("SS up at 59s = %v, want cap stay, no carry", got)
	}

	// MM down preserves seconds; bottom is 00:00:00, never negative.
	m.planned = 10*time.Minute + 90*time.Second // remaining 00:01:30
	m.adjustSel = hmsMM
	m, _ = step(m, keyType(tea.KeyDown))
	if got := m.remaining(); got != 30*time.Second {
		t.Fatalf("MM down from 00:01:30 = %v, want 00:00:30", got)
	}
	m.adjustSel = hmsSS
	for range 30 {
		m, _ = step(m, keyType(tea.KeyDown))
	}
	if got := m.remaining(); got != 0 {
		t.Fatalf("SS down to zero = %v, want 0", got)
	}
	m, _ = step(m, keyType(tea.KeyDown))
	if got := m.remaining(); got != 0 {
		t.Fatalf("SS down at zero = %v, want stay at 0", got)
	}
}

func TestWorkAdjustZeroFinishesOnResume(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	m.planned = 10*time.Minute + time.Minute // remaining 00:01:00
	m.adjustSel = hmsMM
	m, _ = step(m, keyType(tea.KeyDown)) // remaining 00:00:00
	if got := m.remaining(); got != 0 {
		t.Fatalf("setup: remaining = %v, want 0", got)
	}
	m, _ = step(m, keyRune("p")) // resume
	m, _ = step(m, tickMsg(fc.now))
	if m.view != viewPrompt {
		t.Fatal("resuming with zero remaining should prompt via expiry")
	}
}

func TestWorkAdjustKeepsPauseAccounting(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	fc.advance(2 * time.Minute)        // paused wall time
	m, _ = step(m, keyType(tea.KeyUp)) // remaining 50m → 51m
	m, _ = step(m, keyRune("p"))       // resume
	if m.pausedTotal != 2*time.Minute {
		t.Fatalf("pausedTotal = %v, want 2m (adjust adds no pause)", m.pausedTotal)
	}
	fc.advance(5 * time.Minute)
	if got := m.elapsed(); got != 15*time.Minute {
		t.Fatalf("elapsed after resume+5m = %v, want 15m", got)
	}
	if got := m.remaining(); got != 46*time.Minute {
		t.Fatalf("remaining after resume+5m = %v, want 46m", got)
	}
}

func TestWorkAdjustPausedViewHintsKeys(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	if got := m.View(); strings.Contains(got, "up/down") {
		t.Fatalf("running view must not hint adjust keys:\n%s", got)
	}
	m, _ = step(m, keyRune("p"))
	got := m.View()
	for _, want := range []string{"up/down", "left/right", "Paused"} {
		if !strings.Contains(got, want) {
			t.Fatalf("paused view missing %q:\n%s", want, got)
		}
	}
}

func TestWorkAdjustClearsQArm(t *testing.T) {
	m, fc := newTestModel(time.Hour, nil)
	fc.advance(10 * time.Minute)
	m, _ = step(m, keyRune("p"))
	m, _ = step(m, keyRune("q")) // arm while paused
	if !m.qArmed {
		t.Fatal("setup: q should arm abandon confirm")
	}
	m, _ = step(m, keyType(tea.KeyUp))
	if m.qArmed {
		t.Fatal("adjusting must clear a pending q-arm")
	}
}
