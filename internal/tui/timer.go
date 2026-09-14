// Package tui implements the focus terminal UI (workstream B).
//
// timer.go: full-screen Bubble Tea countdown timer. The engine
// (internal/focus Session) owns pause math authoritatively; this model keeps
// its own pausedTotal display accumulator using the same rules (frozen plan
// §5) and returns it in TimerResult for cross-check at persist time.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DefaultPlanned is the session length used when TimerRequest.Planned <= 0
// (frozen plan §5: non-positive planned defaults to 25m).
const DefaultPlanned = 25 * time.Minute

// DefaultBreak is the fresh break length snapshotted every time b opens the
// break editor (captain-settled: 5 minutes, fresh every time).
const DefaultBreak = 5 * time.Minute

// maxBarWidth caps the progress bar width on wide terminals.
const maxBarWidth = 80

// minBarWidth keeps the progress bar usable on narrow terminals.
const minBarWidth = 20

// TimerRequest configures one timer run (frozen plan §10 — exact shape).
type TimerRequest struct {
	Task    string
	Planned time.Duration
	Elapsed func() time.Duration // optional hook; default wall-clock from time.Now
	// BreakDefault overrides the fresh break snapshot length (<=0 →
	// DefaultBreak). Production default stays 5m; trials shorten it via
	// FOCUS_BREAK_SECONDS in the CLI.
	BreakDefault time.Duration
	// OnBreak reports each ended break for persistence (the CLI writes a
	// Kind='break' row). Nil means breaks are not persisted.
	OnBreak func(BreakInfo)
}

// BreakInfo describes one ended break: the chosen (planned) length and the
// wall-clock time actually taken in it.
type BreakInfo struct {
	Planned   time.Duration
	Taken     time.Duration
	StartedAt time.Time
	EndedAt   time.Time
}

// breakField is which HH/MM/SS field the break editor steps.
type breakField int

const (
	breakFieldHH breakField = iota
	breakFieldMM
	breakFieldSS
)

// TimerResult is the outcome of one timer run (frozen plan §10 — exact shape).
type TimerResult struct {
	Completed      bool
	Accomplishment string
	Next           string
	ElapsedTotal   time.Duration
	PausedTotal    time.Duration
}

// RunTimer runs the full-screen timer and blocks until the session is
// finished (Completed=true, with accomplishment/next prompts) or abandoned
// (Completed=false, no prompts). It always returns a usable TimerResult;
// error is reserved for program-level failures.
func RunTimer(req TimerRequest) (TimerResult, error) {
	m := newTimerModel(req)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return TimerResult{}, err
	}
	tm, ok := final.(timerModel)
	if !ok {
		return TimerResult{}, fmt.Errorf("tui: unexpected final model %T", final)
	}
	return tm.result, nil
}

// timerView is which screen the model shows.
type timerView int

const (
	viewTimer timerView = iota
	viewPrompt
	viewBreak
)

// tickMsg drives the 1s countdown. tea.Tick is one-shot: every tickMsg
// re-arms the next tick from Update.
type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type timerModel struct {
	task      string
	planned   time.Duration
	elapsedFn func() time.Duration // gross elapsed hook, nil = wall clock
	now       func() time.Time     // clock for pause accounting (tests inject a fake)
	startTime time.Time

	paused      bool
	pausedAt    time.Time
	pausedTotal time.Duration

	breakDefault time.Duration
	onBreak      func(BreakInfo)

	breakRemain  time.Duration // live countdown value, ticks down once started
	breakPlanned time.Duration // editor-chosen length at start (persisted as planned)
	breakSel     breakField    // field stepped by up/down
	breakStart   time.Time     // break timer start (Taken anchor; zero until started)
	breakTick    time.Time     // last tick seen (wall-delta anchor)
	breakRunning bool          // false = editing (frozen), true = countdown running

	progress progress.Model
	width    int

	view      timerView
	accInput  textinput.Model
	nextInput textinput.Model
	focusIdx  int // 0 = accomplishment, 1 = next

	qArmed bool // first q press arms abandon confirm; second q abandons

	done   bool // result ready; program quits
	result TimerResult

	initCmd tea.Cmd // textinput focus blink, run from Init
}

func newTimerModel(req TimerRequest) timerModel {
	planned := req.Planned
	if planned <= 0 {
		planned = DefaultPlanned
	}
	task := req.Task
	if strings.TrimSpace(task) == "" {
		task = "Untitled session"
	}

	bar := progress.New(progress.WithDefaultGradient(), progress.WithWidth(40))

	acc := textinput.New()
	acc.Placeholder = "what got done"
	acc.Prompt = "> "
	acc.CharLimit = 200

	next := textinput.New()
	next.Placeholder = "the next step"
	next.Prompt = "> "
	next.CharLimit = 200

	focusCmd := acc.Focus()

	return timerModel{
		task:         task,
		planned:      planned,
		elapsedFn:    req.Elapsed,
		breakDefault: req.BreakDefault,
		onBreak:      req.OnBreak,
		now:          time.Now,
		startTime:    time.Now(),
		progress:     bar,
		width:        40,
		view:         viewTimer,
		accInput:     acc,
		nextInput:    next,
		focusIdx:     0,
		initCmd:      focusCmd,
	}
}

// grossElapsed is wall (or hooked) elapsed BEFORE pause subtraction.
// When Elapsed hook is set it must return gross elapsed (like time.Since);
// the §5 pause subtraction below still applies.
func (m timerModel) grossElapsed() time.Duration {
	if m.elapsedFn != nil {
		return m.elapsedFn()
	}
	return m.now().Sub(m.startTime)
}

// elapsed applies the §5 pause-accounting rule:
// Elapsed = gross − pausedTotal − (now − pausedAt if currently paused).
func (m timerModel) elapsed() time.Duration {
	e := m.grossElapsed() - m.pausedTotal
	if m.paused {
		e -= m.now().Sub(m.pausedAt)
	}
	if e < 0 {
		e = 0
	}
	return e
}

// remaining is planned − elapsed; the timer fires done at Remaining <= 0.
func (m timerModel) remaining() time.Duration {
	return m.planned - m.elapsed()
}

// progressPct clamps elapsed/planned into [0,1].
func (m timerModel) progressPct() float64 {
	if m.planned <= 0 {
		return 0
	}
	p := float64(m.elapsed()) / float64(m.planned)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

func (m timerModel) finishResult() TimerResult {
	return TimerResult{
		Completed:      true,
		Accomplishment: strings.TrimSpace(m.accInput.Value()),
		Next:           strings.TrimSpace(m.nextInput.Value()),
		ElapsedTotal:   m.elapsed(),
		PausedTotal:    m.pausedTotal,
	}
}

func (m timerModel) abandonResult() TimerResult {
	return TimerResult{
		Completed:    false,
		ElapsedTotal: m.elapsed(),
		PausedTotal:  m.pausedTotal,
	}
}

// Init arms the 1s tick and the textinput cursor blink.
func (m timerModel) Init() tea.Cmd {
	return tea.Batch(tick(), m.initCmd)
}

// Update routes window/tick/key/frame messages.
func (m timerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w := msg.Width - 4
		if w < minBarWidth {
			w = minBarWidth
		}
		if w > maxBarWidth {
			w = maxBarWidth
		}
		if msg.Width > 0 {
			m.width = w
			m.progress.Width = w
		}
		return m, nil

	case progress.FrameMsg:
		// Never drop progress frames or the bar animation freezes.
		var cmd tea.Cmd
		updated, cmd := m.progress.Update(msg)
		m.progress = updated.(progress.Model)
		return m, cmd

	case tickMsg:
		return m.onTick()

	case tea.KeyMsg:
		if m.view == viewPrompt {
			return m.updatePrompt(msg)
		}
		if m.view == viewBreak {
			return m.updateBreak(msg)
		}
		return m.updateTimer(msg)
	}
	return m, nil
}

func (m timerModel) onTick() (tea.Model, tea.Cmd) {
	if m.done {
		return m, tea.Quit
	}
	if m.view == viewBreak {
		if !m.breakRunning {
			// Editing phase: countdown frozen until enter/s starts it.
			// The work session stays auto-paused underneath.
			pctCmd := m.progress.SetPercent(m.progressPct())
			return m, tea.Batch(tick(), pctCmd)
		}
		// Running break ticks wall-clock delta from start; the work
		// session stays auto-paused underneath (elapsed math frozen).
		now := m.now()
		m.breakRemain -= now.Sub(m.breakTick)
		m.breakTick = now
		if m.breakRemain <= 0 {
			m.finishBreak()
		}
		pctCmd := m.progress.SetPercent(m.progressPct())
		return m, tea.Batch(tick(), pctCmd)
	}
	if m.view == viewTimer && m.remaining() <= 0 {
		m.enterPrompt()
		return m, nil
	}
	pctCmd := m.progress.SetPercent(m.progressPct())
	return m, tea.Batch(tick(), pctCmd)
}

// updateTimer handles keys in the countdown view.
func (m timerModel) updateTimer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.settlePause()
		m.done = true
		m.result = m.abandonResult()
		return m, tea.Quit
	case "p":
		m.qArmed = false
		if m.paused {
			// Resume: accumulate exactly the paused interval (§5).
			m.pausedTotal += m.now().Sub(m.pausedAt)
			m.paused = false
		} else {
			m.paused = true
			m.pausedAt = m.now()
		}
		return m, nil
	case "s":
		m.qArmed = false
		m.enterPrompt()
		return m, nil
	case "q":
		if m.qArmed {
			m.settlePause()
			m.done = true
			m.result = m.abandonResult()
			return m, tea.Quit
		}
		m.qArmed = true
		return m, nil
	case "b":
		m.enterBreak()
		return m, nil
	default:
		m.qArmed = false
		return m, nil
	}
}

// settlePause folds a pending pause interval into pausedTotal (§5
// auto-resume accounting) so finish/abandon from paused state stay honest.
func (m *timerModel) settlePause() {
	if m.paused {
		m.pausedTotal += m.now().Sub(m.pausedAt)
		m.paused = false
	}
}

// enterBreak snapshots a fresh break (DefaultBreak unless BreakDefault
// overrides), selects the minutes field, and auto-pauses the work session —
// reusing the paused math untouched, so elapsed/remaining freeze exactly as
// if p had been pressed. The break countdown stays frozen in the editor
// until enter/s starts it (breakRunning=false). Any pending q-arm is
// cleared: the break is a transient editor, and abandoning it must never be
// one keypress away.
func (m *timerModel) enterBreak() {
	d := m.breakDefault
	if d <= 0 {
		d = DefaultBreak
	}
	now := m.now()
	m.breakRemain = d
	m.breakPlanned = d
	m.breakSel = breakFieldMM
	m.breakStart = time.Time{}
	m.breakTick = now
	m.breakRunning = false
	if !m.paused {
		m.paused = true
		m.pausedAt = now
	}
	m.qArmed = false
	m.view = viewBreak
}

// startBreak moves the break from the frozen editor into the running
// countdown: the chosen length freezes as planned and the Taken anchor
// starts here, so editor idle time is work-pause but not break time.
func (m *timerModel) startBreak() {
	now := m.now()
	m.breakPlanned = m.breakRemain
	m.breakStart = now
	m.breakTick = now
	m.breakRunning = true
}

// finishBreak ends the break (expiry, enter, or s): resumes work with the
// whole break wall interval folded into pausedTotal, and reports the break
// for persistence — unless it was zero-length (planned or taken empty),
// which persists nothing.
func (m *timerModel) finishBreak() {
	now := m.now()
	taken := now.Sub(m.breakStart)
	planned := m.breakPlanned
	m.settlePause()
	m.view = viewTimer
	if planned > 0 && taken > 0 && m.onBreak != nil {
		m.onBreak(BreakInfo{
			Planned:   planned,
			Taken:     taken,
			StartedAt: m.breakStart,
			EndedAt:   now,
		})
	}
}

// cancelBreak drops the break (esc, q, or enter/s on 00:00): resumes work
// with the interval folded into pausedTotal, persists nothing.
func (m *timerModel) cancelBreak() {
	m.settlePause()
	m.view = viewTimer
}

func clampField(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// stepBreakField steps the selected HH/MM/SS field by dh hours / dm minutes /
// ds seconds with Qt section semantics (research §2.2): each field saturates
// independently — clamp everywhere, no carry, no wrap.
func (m *timerModel) stepBreakField(dh, dm, ds int) {
	total := m.breakRemain
	if total < 0 {
		total = 0
	}
	sec := int((total % time.Minute) / time.Second)
	h := int(total / time.Hour)
	min := int((total % time.Hour) / time.Minute)
	switch m.breakSel {
	case breakFieldHH:
		h = clampField(h+dh, 0, 23)
	case breakFieldMM:
		min = clampField(min+dm, 0, 59)
	default:
		sec = clampField(sec+ds, 0, 59)
	}
	m.breakRemain = time.Duration(h)*time.Hour + time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
	m.breakPlanned = m.breakRemain
}

// arrowKey normalizes arrow input to "up"/"down"/"left"/"right", accepting
// both key spellings (msg.String() and msg.Type per research §1.1). Both
// cursor-key escape forms (CSI `ESC[A` and SS3 `ESC O A` per §1.2) already
// decode to the same KeyMsg upstream, so they arrive here identically.
// Anything else returns "".
func arrowKey(msg tea.KeyMsg) string {
	switch msg.String() {
	case "up", "down", "left", "right":
		return msg.String()
	}
	switch msg.Type {
	case tea.KeyUp:
		return "up"
	case tea.KeyDown:
		return "down"
	case tea.KeyLeft:
		return "left"
	case tea.KeyRight:
		return "right"
	}
	return ""
}

// updateBreak handles keys in the break editor/countdown view. Editing phase
// (frozen): up/down step the selected HH/MM/SS field, left/right move
// between the three fields (stopping at the ends), enter/s start the
// countdown (00:00:00 cancels instead), esc/q cancel (persist nothing).
// Running phase: the countdown ticks; enter/s end it early (persisting),
// esc/q cancel (persist nothing), arrows are ignored. ctrl+c keeps the timer
// abandon semantics. b does nothing during a break (no re-snapshot); p is a
// no-op (work is already auto-paused).
func (m timerModel) updateBreak(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if dir := arrowKey(msg); dir != "" {
		if m.breakRunning {
			return m, nil
		}
		switch dir {
		case "up":
			switch m.breakSel {
			case breakFieldHH:
				m.stepBreakField(1, 0, 0)
			case breakFieldMM:
				m.stepBreakField(0, 1, 0)
			default:
				m.stepBreakField(0, 0, 1)
			}
		case "down":
			switch m.breakSel {
			case breakFieldHH:
				m.stepBreakField(-1, 0, 0)
			case breakFieldMM:
				m.stepBreakField(0, -1, 0)
			default:
				m.stepBreakField(0, 0, -1)
			}
		case "left":
			if m.breakSel == breakFieldSS {
				m.breakSel = breakFieldMM
			} else {
				m.breakSel = breakFieldHH
			}
		case "right":
			if m.breakSel == breakFieldHH {
				m.breakSel = breakFieldMM
			} else {
				m.breakSel = breakFieldSS
			}
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c":
		m.settlePause()
		m.done = true
		m.result = m.abandonResult()
		return m, tea.Quit
	case "enter", "s":
		if !m.breakRunning {
			// A zero-length choice cancels instead of starting an
			// empty break row.
			if m.breakRemain <= 0 {
				m.cancelBreak()
			} else {
				m.startBreak()
			}
			return m, nil
		}
		m.finishBreak()
		return m, nil
	case "esc", "q":
		m.cancelBreak()
		return m, nil
	default:
		return m, nil
	}
}

// enterPrompt switches to the finish view and focuses the first field.
func (m *timerModel) enterPrompt() {
	m.settlePause()
	m.view = viewPrompt
	m.focusIdx = 0
	m.accInput.Focus()
	m.nextInput.Blur()
}

// updatePrompt handles keys in the accomplishment/next view.
func (m timerModel) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.settlePause()
		m.done = true
		m.result = m.abandonResult()
		return m, tea.Quit
	case "esc":
		// Back to the timer (only useful pre-expiry; post-expiry the
		// next tick re-prompts since Remaining is still <= 0).
		m.view = viewTimer
		m.accInput.Blur()
		m.nextInput.Blur()
		return m, nil
	case "tab", "shift+tab":
		if m.focusIdx == 0 {
			m.focusIdx = 1
			m.accInput.Blur()
			m.nextInput.Focus()
		} else {
			m.focusIdx = 0
			m.nextInput.Blur()
			m.accInput.Focus()
		}
		return m, nil
	case "enter":
		if m.focusIdx == 0 {
			m.focusIdx = 1
			m.accInput.Blur()
			m.nextInput.Focus()
			return m, nil
		}
		m.done = true
		m.result = m.finishResult()
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		if m.focusIdx == 0 {
			m.accInput, cmd = m.accInput.Update(msg)
		} else {
			m.nextInput, cmd = m.nextInput.Update(msg)
		}
		return m, cmd
	}
}

// Shared look and feel (also used by history.go / stats.go — same package).
var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	styleTime     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleStatus   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	stylePaused   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	styleFooter   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBreakSel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	styleKey      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleBad      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
)

// fmtHMS renders d as HH:MM:SS, clamping negatives to 00:00:00.
func fmtHMS(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d.Round(time.Second).Seconds())
	h, rem := total/3600, total%3600
	return fmt.Sprintf("%02d:%02d:%02d", h, rem/60, rem%60)
}

// fmtDur renders d compactly for lists: 2h5m, 25m, 25m30s, 45s.
func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < 0 {
		d = 0
	}
	h := d / time.Hour
	d -= h * time.Hour
	min := d / time.Minute
	d -= min * time.Minute
	sec := d / time.Second
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, min)
	case min > 0:
		if sec > 0 {
			return fmt.Sprintf("%dm%ds", min, sec)
		}
		return fmt.Sprintf("%dm", min)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

// View renders the current screen.
func (m timerModel) View() string {
	if m.view == viewPrompt {
		return m.viewPrompt()
	}
	if m.view == viewBreak {
		return m.viewBreak()
	}
	return m.viewTimer()
}

func (m timerModel) viewBreak() string {
	var b strings.Builder

	title := "Break — set length"
	if m.breakRunning {
		title = "Break — running"
	}
	sub := "session paused · " + m.task
	b.WriteString(styleTitle.Render(title) + "\n\n")
	b.WriteString(m.renderBreakClock() + "\n\n")
	b.WriteString(styleDim.Render(sub) + "\n\n")

	var footer string
	if m.breakRunning {
		footer = styleKey.Render("enter") + styleFooter.Render(" end break · ") +
			styleKey.Render("s") + styleFooter.Render(" end early · ") +
			styleKey.Render("esc") + styleFooter.Render(" cancel")
	} else {
		footer = styleKey.Render("up/down") + styleFooter.Render(" adjust · ") +
			styleKey.Render("left/right") + styleFooter.Render(" field · ") +
			styleKey.Render("enter") + styleFooter.Render(" start break · ") +
			styleKey.Render("s") + styleFooter.Render(" start · ") +
			styleKey.Render("esc") + styleFooter.Render(" cancel")
	}
	b.WriteString(footer)

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

// renderBreakClock renders the break editor/remaining time as hh:mm:ss. In
// the frozen editor the selected HH/MM/SS field is highlighted; once running
// the clock renders plain (editing locked).
func (m timerModel) renderBreakClock() string {
	d := m.breakRemain
	if d < 0 {
		d = 0
	}
	total := int64(d.Round(time.Second).Seconds())
	hh := fmt.Sprintf("%02d", total/3600)
	mm := fmt.Sprintf("%02d", (total%3600)/60)
	ss := fmt.Sprintf("%02d", total%60)
	hs, ms, sss := styleTime.Render(hh), styleTime.Render(mm), styleTime.Render(ss)
	if !m.breakRunning {
		switch m.breakSel {
		case breakFieldHH:
			hs = styleBreakSel.Render(hh)
		case breakFieldMM:
			ms = styleBreakSel.Render(mm)
		default:
			sss = styleBreakSel.Render(ss)
		}
	}
	return hs + styleTime.Render(":") + ms + styleTime.Render(":") + sss
}

func (m timerModel) viewTimer() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render(m.task) + "\n\n")

	b.WriteString(styleLabel.Render("Elapsed   ") + styleTime.Render(fmtHMS(m.elapsed())) + "\n")
	b.WriteString(styleLabel.Render("Remaining ") + styleTime.Render(fmtHMS(m.remaining())) + "\n\n")

	b.WriteString(m.progress.ViewAs(m.progressPct()) + "\n\n")

	status := styleStatus.Render("Running")
	if m.paused {
		status = stylePaused.Render("Paused (timer held)")
	}
	b.WriteString(styleLabel.Render("Status    ") + status + "\n\n")

	if m.qArmed {
		b.WriteString(styleBad.Render("Press q again to abandon this session.") + "\n\n")
	}

	footer := styleKey.Render("p") + styleFooter.Render(" pause/resume · ") +
		styleKey.Render("s") + styleFooter.Render(" finish · ") +
		styleKey.Render("q") + styleFooter.Render(" abandon · ") +
		styleKey.Render("b") + styleFooter.Render(" break")
	b.WriteString(footer)

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m timerModel) viewPrompt() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render("Session complete — "+m.task) + "\n")
	b.WriteString(styleDim.Render(fmt.Sprintf("%s focused in this session.", fmtDur(m.elapsed()))) + "\n\n")

	b.WriteString(styleLabel.Render("What did you accomplish?") + "\n")
	b.WriteString(m.accInput.View() + "\n\n")

	b.WriteString(styleLabel.Render("What is the next step?") + "\n")
	b.WriteString(m.nextInput.View() + "\n\n")

	footer := styleKey.Render("tab") + styleFooter.Render(" switch field · ") +
		styleKey.Render("enter") + styleFooter.Render(" continue / finish · ") +
		styleKey.Render("esc") + styleFooter.Render(" back")
	b.WriteString(footer)

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}
