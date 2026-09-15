package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/focus-cli/focus/internal/tui"
)

func newHeatmapCmd() *cobra.Command {
	var days int
	cmd := &cobra.Command{
		Use:   "heatmap",
		Short: "Show when you are most productive",
		Long: `Show a weekday × hour heatmap of completed focus sessions.

Each cell accumulates honest focused time (wall-clock minus paused) over
the trailing window, with sessions spanning hour boundaries split
proportionally. More blocks mean more focus relative to your hottest
slot. Breaks and abandoned sessions are excluded, consistent with stats.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if days <= 0 {
				return UsageError{fmt.Errorf("--days must be a positive integer, got %d", days)}
			}
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
			grid := tui.BuildProductivityGrid(records, time.Now(), days)
			fmt.Fprint(cmd.OutOrStdout(), tui.RenderProductivityGrid(grid))
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 30, "trailing days to include")
	return cmd
}
