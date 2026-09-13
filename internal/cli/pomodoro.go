// Package cli — `focus pomodoro`: a repeating work/break rotation on top of
// the Phase 1 timer. Each work block runs the full-screen timer (or a
// headless countdown) and persists as a normal session row, so history and
// stats keep working untouched; breaks are never persisted and are
// skippable with `s`.
package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/focus-cli/focus/internal/focus"
	"github.com/focus-cli/focus/internal/tui"
)

// phaseRunner executes one timer phase (work or break).
type phaseRunner func(ctx context.Context, req tui.TimerRequest) (tui.TimerResult, error)

// runPomodoro drives the rotation: work blocks persist as normal session
// rows; breaks just pass time. It returns how many work blocks completed.
// maxBlocks > 0 stops the rotation after that many work blocks (0 = run
// until the user abandons a work block, quits a break, or cancels).
// clock may be nil (wall clock); tests inject a fake via the cycle.
func runPomodoro(ctx context.Context, store focus.Store, out io.Writer, task string, cfg focus.PomodoroConfig, maxBlocks int, clock focus.Clock, run phaseRunner) (done int, err error) {
	cycle := focus.NewPomodoroCycle(cfg, clock)
	for {
		if err := ctx.Err(); err != nil {
			return done, err
		}
		switch cycle.Phase() {
		case focus.PomodoroWork:
			id, err := store.Create(ctx, focus.SessionRecord{
				Task:           task,
				PlannedSeconds: int64(cycle.PhaseDuration() / time.Second),
				StartedAt:      time.Now(),
			})
			if err != nil {
				return done, err
			}
			res, err := run(ctx, tui.TimerRequest{
				Task:       task,
				Planned:    cycle.PhaseDuration(),
				PhaseLabel: cycle.PhaseLabel(),
			})
			if err != nil {
				return done, err
			}
			if err := store.Complete(ctx, id, time.Now(), int64(res.PausedTotal/time.Second), res.Accomplishment, res.Next, res.Completed); err != nil {
				return done, err
			}
			if !res.Completed {
				fmt.Fprintf(out, "Abandoned: %s (kept in history, out of stats)\n", task)
				return done, nil
			}
			done++
			fmt.Fprintf(out, "Work block %d done: %s\n", done, task)
			if maxBlocks > 0 && done >= maxBlocks {
				fmt.Fprintf(out, "Pomodoro complete: %d work blocks on %q.\n", done, task)
				return done, nil
			}
		default: // short or long break: never persisted
			res, err := run(ctx, tui.TimerRequest{
				Task:       task,
				Planned:    cycle.PhaseDuration(),
				PhaseLabel: cycle.PhaseLabel(),
				Break:      true,
			})
			if err != nil {
				return done, err
			}
			if !res.Completed {
				fmt.Fprintf(out, "Pomodoro stopped after %d work blocks.\n", done)
				return done, nil
			}
			fmt.Fprintf(out, "%s over — back to work.\n", cycle.PhaseLabel())
		}
		cycle.Advance()
	}
}

func newPomodoroCmd() *cobra.Command {
	var blocks, workMin, shortMin, longMin, every int
	var noTUI bool
	cmd := &cobra.Command{
		Use:   "pomodoro \"<task>\"",
		Short: "Run a pomodoro rotation on a task",
		Long: `Run a repeating work/break rotation on a task: work blocks with short
breaks and a long break after every set.

Each work block runs the full-screen timer and is saved as a normal session
(history and stats keep working). Breaks are rest time: nothing is saved and
s skips to the next work block.

Keys (timer): p pause/resume · s finish work / skip break ·
q abandon work block / quit rotation (double-press).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if blocks < 0 {
				return UsageError{fmt.Errorf("--blocks must be zero (until quit) or a positive integer, got %d", blocks)}
			}
			for name, val := range map[string]int{
				"work": workMin, "short-break": shortMin, "long-break": longMin, "every": every,
			} {
				if cmd.Flags().Changed(name) && val <= 0 {
					return UsageError{fmt.Errorf("--%s must be a positive integer number of minutes/blocks, got %d", name, val)}
				}
			}

			ctx := cmd.Context()
			task := args[0]
			store, cfg, err := openStore(ctx)
			if err != nil {
				return err
			}
			defer store.Close()

			pcfg := focus.PomodoroConfig{
				Work:             cfg.PomodoroWork,
				ShortBreak:       cfg.PomodoroShortBreak,
				LongBreak:        cfg.PomodoroLongBreak,
				BlocksBeforeLong: cfg.PomodoroBlocksBeforeLong,
			}
			if cmd.Flags().Changed("work") {
				pcfg.Work = time.Duration(workMin) * time.Minute
			}
			if cmd.Flags().Changed("short-break") {
				pcfg.ShortBreak = time.Duration(shortMin) * time.Minute
			}
			if cmd.Flags().Changed("long-break") {
				pcfg.LongBreak = time.Duration(longMin) * time.Minute
			}
			if cmd.Flags().Changed("every") {
				pcfg.BlocksBeforeLong = every
			}

			out := cmd.OutOrStdout()
			headless := noTUI || !isTTY()
			run := func(ctx context.Context, req tui.TimerRequest) (tui.TimerResult, error) {
				if !headless {
					return tui.RunTimer(req)
				}
				fmt.Fprintf(out, "=== %s (%s) ===\n", req.PhaseLabel, compactDur(req.Planned))
				if err := countdown(ctx, out, req.Planned); err != nil {
					return tui.TimerResult{}, err
				}
				return tui.TimerResult{Completed: true, ElapsedTotal: req.Planned}, nil
			}

			_, err = runPomodoro(ctx, store, out, task, pcfg, blocks, nil, run)
			return err
		},
	}
	cmd.Flags().IntVar(&blocks, "blocks", 0, "stop after N work blocks (0 = until quit)")
	cmd.Flags().IntVar(&workMin, "work", 0, "work block minutes (default: env or 25)")
	cmd.Flags().IntVar(&shortMin, "short-break", 0, "short break minutes (default: env or 5)")
	cmd.Flags().IntVar(&longMin, "long-break", 0, "long break minutes (default: env or 15)")
	cmd.Flags().IntVar(&every, "every", 0, "work blocks per set before a long break (default: env or 4)")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "run plain headless countdowns instead of the full-screen timer")
	return cmd
}

// compactDur renders whole-minute durations as "25m" (sub-minute falls back
// to the standard form) for headless phase headers.
func compactDur(d time.Duration) string {
	if d <= 0 {
		return d.String()
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}
