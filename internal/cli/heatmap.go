package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/focus-cli/focus/internal/tui"
)

func newHeatmapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heatmap",
		Short: "Show this week's focus activity",
		Long: `Show a 7-day activity heatmap of completed focus sessions.

The rolling window covers the last 7 local days including today. Each day
shows its total honest focused time (wall-clock minus paused) and session
count; breaks and abandoned sessions are excluded, consistent with stats.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, _, err := openStore(ctx)
			if err != nil {
				return err
			}
			defer store.Close()
			records, err := store.List(ctx, 0)
			if err != nil {
				return err
			}
			days := tui.BuildWeekHeat(records, time.Now())
			fmt.Fprint(cmd.OutOrStdout(), tui.RenderHeatmap(days))
			return nil
		},
	}
	return cmd
}
