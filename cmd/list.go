package cmd

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/db"
	"github.com/timae/rel.ai/internal/display"
)

var (
	listSince   string
	listUntil   string
	listProject string
	listSource  string
	listTag     string
	listLimit   int
	listAll     bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Browse sessions",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		f := db.ListFilter{Limit: listLimit}

		if listAll {
			f.Limit = 0
		}

		if listSince != "" {
			t, err := parseDate(listSince)
			if err != nil {
				return err
			}
			f.Since = &t
		}
		if listUntil != "" {
			t, err := parseDate(listUntil)
			if err != nil {
				return err
			}
			f.Until = &t
		}
		f.Project = listProject
		f.Source = listSource
		f.Tag = listTag

		sessions, err := store.ListSessions(f)
		if err != nil {
			return err
		}

		display.SessionTable(sessions)
		return nil
	},
}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

func init() {
	listCmd.Flags().StringVar(&listSince, "since", "", "show sessions after date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listUntil, "until", "", "show sessions before date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listProject, "project", "", "filter by project path (substring)")
	listCmd.Flags().StringVar(&listSource, "source", "", "filter by source (claude|codex)")
	listCmd.Flags().StringVar(&listTag, "tag", "", "filter by tag")
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "max sessions to show")
	listCmd.Flags().BoolVar(&listAll, "all", false, "show all sessions (no limit)")
	rootCmd.AddCommand(listCmd)
}
