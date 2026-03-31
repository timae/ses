package cmd

import (
	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/display"
)

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show session details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := store.GetSession(args[0])
		if err != nil {
			return err
		}
		display.SessionDetail(session)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
