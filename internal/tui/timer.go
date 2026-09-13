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

// maxBarWidth caps the progress bar width on wide terminals.
const maxBarWidth = 80

// minBarWidth keeps the progress bar usable on narrow terminals.
const minBarWidth = 20

// TimerRequest configures one timer run (frozen plan §10 — exact shape,
// plus optional pomodoro fields, zero-valued for plain sessions).
type TimerRequest struct {
	Task    string
	Planned time.Duration
	Elapsed func() time.Duration // optional hook; default wall-clock from time.Now
	// PhaseLabel names the pomodoro phase in the timer view
	// (e.g. "Work 1 of 4", "Short break"); empty hides the line.
	PhaseLabel string
	// Break marks a rest phase: no accomplishment prompts — s skips the
	// break and expiry auto-completes (both Completed=true); double-q
	// quits (Completed=false).
	Break bool
}

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
	task       string
	planned    time.Duration
	elapsedFn  func() time.Duration // gross elapsed hook, nil = wall clock
	phaseLabel string
	isBreak    bool
	now        func() time.Time // clock for pause accounting (tests inject a fake)
	startTime  time.Time

	paused      bool
	pausedAt    time.Time
	pausedTotal time.Duration

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
		task:       task,
		planned:    planned,
		elapsedFn:  req.Elapsed,
		phaseLabel: req.PhaseLabel,
		isBreak:    req.Break,
		now:        time.Now,
		startTime:  time.Now(),
		progress:   bar,
		width:      40,
		view:       viewTimer,
		accInput:   acc,
		nextInput:  next,
		focusIdx:   0,
		initCmd:    focusCmd,
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

// skipResult ends a break phase: the break is over (skipped or elapsed) so
// the rotation continues. It carries no accomplishment prompts.
func (m timerModel) skipResult() TimerResult {
	return TimerResult{
		Completed:    true,
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
		return m.updateTimer(msg)
	}
	return m, nil
}

func (m timerModel) onTick() (tea.Model, tea.Cmd) {
	if m.done {
		return m, tea.Quit
	}
	if m.view == viewTimer && m.remaining() <= 0 {
		if m.isBreak {
			m.settlePause()
			m.done = true
			m.result = m.skipResult()
			return m, tea.Quit
		}
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
		if m.isBreak {
			m.settlePause()
			m.done = true
			m.result = m.skipResult()
			return m, tea.Quit
		}
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
	styleTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	styleTime   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleStatus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	stylePaused = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	styleFooter = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleKey    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	styleOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleBad    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
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
	return m.viewTimer()
}

func (m timerModel) viewTimer() string {
	var b strings.Builder

	b.WriteString(styleTitle.Render(m.task) + "\n")
	if strings.TrimSpace(m.phaseLabel) != "" {
		b.WriteString(styleLabel.Render(strings.TrimSpace(m.phaseLabel)) + "\n")
	}
	b.WriteString("\n")

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

	var footer string
	if m.isBreak {
		footer = styleKey.Render("p") + styleFooter.Render(" pause/resume · ") +
			styleKey.Render("s") + styleFooter.Render(" skip break · ") +
			styleKey.Render("q") + styleFooter.Render(" quit")
	} else {
		footer = styleKey.Render("p") + styleFooter.Render(" pause/resume · ") +
			styleKey.Render("s") + styleFooter.Render(" finish · ") +
			styleKey.Render("q") + styleFooter.Render(" abandon")
	}
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
