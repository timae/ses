package cmd

import (
	"github.com/spf13/cobra"
	"github.com/timae/ses/internal/db"
	"github.com/timae/ses/internal/display"
)

var (
	statsSince   string
	statsUntil   string
	statsProject string
	statsSource  string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show session analytics dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		f := db.ListFilter{}

		if statsSince != "" {
			t, err := parseDate(statsSince)
			if err != nil {
				return err
			}
			f.Since = &t
		}
		if statsUntil != "" {
			t, err := parseDate(statsUntil)
			if err != nil {
				return err
			}
			f.Until = &t
		}
		f.Project = statsProject
		f.Source = statsSource

		stats, err := store.GetStats(f)
		if err != nil {
			return err
		}

		display.StatsDashboard(stats)
		return nil
	},
}

func init() {
	statsCmd.Flags().StringVar(&statsSince, "since", "", "stats from date (YYYY-MM-DD)")
	statsCmd.Flags().StringVar(&statsUntil, "until", "", "stats until date (YYYY-MM-DD)")
	statsCmd.Flags().StringVar(&statsProject, "project", "", "filter by project (substring)")
	statsCmd.Flags().StringVar(&statsSource, "source", "", "filter by source (claude|codex)")
	rootCmd.AddCommand(statsCmd)
}
