// Package cli wires cobra commands to config, storage, and the TUI.
// Parsing lives here (workstream A); rendering helpers and the timer model
// live in internal/tui (workstream B) behind the frozen §10 API.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/focus-cli/focus/internal/config"
	"github.com/focus-cli/focus/internal/focus"
	"github.com/focus-cli/focus/internal/storage"
	"github.com/focus-cli/focus/internal/tui"
)

// UsageError marks exit code 2 (usage error). Runtime failures exit 1.
type UsageError struct{ Err error }

func (e UsageError) Error() string { return e.Err.Error() }
func (e UsageError) Unwrap() error { return e.Err }

// NewRootCmd builds the `focus` command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "focus",
		Short:         "Terminal focus operating system",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newStartCmd(), newPomodoroCmd(), newHistoryCmd(), newStatsCmd(), newHeatmapCmd())
	return root
}

// Execute runs the root command (thin wrapper for main).
func Execute() error {
	return NewRootCmd().Execute()
}

// openStore loads config, opens the DB, and migrates. Callers close it.
func openStore(ctx context.Context) (*storage.SQLiteStore, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, err
	}
	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return nil, config.Config{}, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, config.Config{}, err
	}
	return store, cfg, nil
}

func newStartCmd() *cobra.Command {
	var noTUI bool
	cmd := &cobra.Command{
		Use:   "start \"<task>\" [minutes]",
		Short: "Start a focus session",
		Long: `Start a focus session with a full-screen countdown timer.

Keys: p pause/resume · s finish (prompts for accomplishment + next step) ·
b break · q abandon (kept in history, excluded from stats) ·
arrows adjust remaining time while paused.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			task := args[0]

			store, cfg, err := openStore(ctx)
			if err != nil {
				return err
			}
			defer store.Close()

			planned := cfg.DefaultLength
			if len(args) == 2 {
				n, err := strconv.Atoi(args[1])
				if err != nil || n <= 0 {
					return UsageError{fmt.Errorf("minutes must be a positive integer, got %q", args[1])}
				}
				planned = time.Duration(n) * time.Minute
			}

			id, err := store.Create(ctx, focus.SessionRecord{
				Task:           task,
				PlannedSeconds: int64(planned / time.Second),
				StartedAt:      time.Now(),
			})
			if err != nil {
				return err
			}

			var res tui.TimerResult
			if noTUI || !isTTY() {
				res, err = runHeadless(cmd, planned)
			} else {
				res, err = tui.RunTimer(tui.TimerRequest{
					Task:         task,
					Planned:      planned,
					BreakDefault: breakDefaultFromEnv(),
					OnBreak:      breakPersistHook(ctx, store, task),
				})
			}
			if err != nil {
				return err
			}

			endedAt := time.Now()
			pausedSec := int64(res.PausedTotal / time.Second)
			if err := store.Complete(ctx, id, endedAt, pausedSec, res.Accomplishment, res.Next, res.Completed); err != nil {
				return err
			}
			if res.Completed {
				fmt.Fprintf(cmd.OutOrStdout(), "Done: %s\n", task)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Abandoned: %s (kept in history, out of stats)\n", task)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "run a plain headless countdown instead of the full-screen timer")
	return cmd
}

func newHistoryCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List past sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return UsageError{fmt.Errorf("--limit must be a positive integer, got %d", limit)}
			}
			ctx := cmd.Context()
			store, _, err := openStore(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			records, err := store.List(ctx, limit)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), tui.RenderHistory(records, limit))
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max sessions to show")
	return cmd
}

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show today's total, count, and average",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, _, err := openStore(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			now := time.Now()
			total, count, avg, err := store.StatsToday(ctx, now)
			if err != nil {
				return err
			}
			// Per-session lines for today: filter a recent window by local day.
			recent, err := store.List(ctx, 1000)
			if err != nil {
				return err
			}
			y, m, d := now.Date()
			dayStart := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
			// Per-session lines for today: completed focus rows only. Totals
			// already exclude breaks (StatsToday counts kind='focus'); this
			// keeps the lines consistent. (Workstream A cannot touch the CLI
			// per its scope, so the guard lives here with the wiring.)
			var today []focus.SessionRecord
			for _, r := range recent {
				if r.Completed && r.Kind != focus.KindBreak && !r.StartedAt.Before(dayStart) && r.StartedAt.Before(dayStart.Add(24*time.Hour)) {
					today = append(today, r)
				}
			}
			fmt.Fprint(cmd.OutOrStdout(), tui.RenderStats(total, count, avg, today))
			return nil
		},
	}
	return cmd
}

// isTTY reports whether stdout is a terminal (stdlib-only check).
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// runHeadless runs a plain ticking countdown on non-TTY outputs (or
// --no-tui), then auto-finishes with empty prompts. It blocks for the full
// planned duration.
func runHeadless(cmd *cobra.Command, planned time.Duration) (tui.TimerResult, error) {
	if err := countdown(cmd.Context(), cmd.OutOrStdout(), planned); err != nil {
		return tui.TimerResult{}, err
	}
	return tui.TimerResult{Completed: true, ElapsedTotal: planned}, nil
}

// countdown blocks for the full planned duration, ticking once a second.
// Shared by `start --no-tui` and the headless pomodoro phases.
func countdown(ctx context.Context, out interface {
	Write([]byte) (int, error)
}, planned time.Duration) error {
	deadline := time.Now().Add(planned)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		rem := time.Until(deadline).Round(time.Second)
		if rem <= 0 {
			break
		}
		fmt.Fprintf(out, "\rremaining %s", rem)
		select {
		case <-ctx.Done():
			fmt.Fprintln(out)
			return ctx.Err()
		case <-ticker.C:
		}
	}
	fmt.Fprintln(out)
	return nil
}

// breakDefaultFromEnv reads FOCUS_BREAK_SECONDS (a trial hook): a positive
// integer overrides the 5-minute break default with that many seconds.
// Empty or garbage → 0, and the TUI falls back to DefaultBreak.
func breakDefaultFromEnv() time.Duration {
	raw := os.Getenv("FOCUS_BREAK_SECONDS")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// breakPersistHook returns the TUI OnBreak callback: each ended break is
// persisted immediately as a completed Kind='break' row (visible in history,
// excluded from stats), so the row survives even if the session is later
// abandoned. A persist failure is reported on stderr without killing the
// session — the work timer is the authority, the break row is a record.
func breakPersistHook(ctx context.Context, store *storage.SQLiteStore, task string) func(tui.BreakInfo) {
	return func(b tui.BreakInfo) {
		ended := b.EndedAt
		if ended.IsZero() {
			ended = time.Now()
		}
		_, err := store.Create(ctx, focus.SessionRecord{
			Task:           task,
			Kind:           focus.KindBreak,
			PlannedSeconds: int64(b.Planned / time.Second),
			StartedAt:      b.StartedAt,
			EndedAt:        &ended,
			Completed:      true,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "focus: could not persist break row: %v\n", err)
		}
	}
}

// ExitCode maps an Execute error to the process exit code: 0 ok, 2 usage, 1 runtime.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ue UsageError
	if errors.As(err, &ue) {
		return 2
	}
	return 1
}
