package cmd

import (
	"fmt"

	"github.com/fatih/color"
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

		// Show linked sessions
		linked, err := store.GetLinkedSessions(args[0])
		if err == nil && len(linked) > 0 {
			fmt.Println()
			color.New(color.Bold).Println("Linked Sessions")
			for _, ls := range linked {
				arrow := "→"
				if ls.Direction == "from" {
					arrow = "←"
				}
				prompt := display.Truncate(ls.FirstPrompt, 50)
				reason := ""
				if ls.Reason != "" {
					reason = fmt.Sprintf(" (%s)", ls.Reason)
				}
				fmt.Printf("  %s %s %s — %s%s\n",
					arrow, ls.ShortID[:8], ls.SourceType, prompt, reason)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
