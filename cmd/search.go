package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/timae/ses/internal/display"
)

var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Full-text search across sessions",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		sessions, err := store.Search(query, searchLimit)
		if err != nil {
			return err
		}
		display.SessionTable(sessions)
		return nil
	},
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "max results")
	rootCmd.AddCommand(searchCmd)
}
