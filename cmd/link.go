package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/display"
)

var (
	linkReason string
	linkRemove bool
	linkList   bool
)

var linkCmd = &cobra.Command{
	Use:   "link <id1> [id2]",
	Short: "Link related sessions together",
	Long:  "Chain sessions that are part of the same task.\nUse --list to show all linked sessions.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// List mode
		if linkList || len(args) == 1 {
			linked, err := store.GetLinkedSessions(args[0])
			if err != nil {
				return err
			}
			if len(linked) == 0 {
				fmt.Println("No linked sessions.")
				return nil
			}

			bold := color.New(color.Bold)
			bold.Printf("Sessions linked to %s:\n\n", args[0])
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
			return nil
		}

		// Link or unlink two sessions
		if linkRemove {
			if err := store.RemoveLink(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Unlinked %s ↔ %s\n", args[0], args[1])
		} else {
			if err := store.AddLink(args[0], args[1], linkReason); err != nil {
				return err
			}
			msg := fmt.Sprintf("Linked %s → %s", args[0], args[1])
			if linkReason != "" {
				msg += fmt.Sprintf(" (%s)", linkReason)
			}
			fmt.Println(msg)
		}

		return nil
	},
}

func init() {
	linkCmd.Flags().StringVar(&linkReason, "reason", "", "reason for linking")
	linkCmd.Flags().BoolVar(&linkRemove, "remove", false, "remove link between sessions")
	linkCmd.Flags().BoolVar(&linkList, "list", false, "list linked sessions")
	rootCmd.AddCommand(linkCmd)
}
