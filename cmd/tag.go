package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var tagRemove bool

var tagCmd = &cobra.Command{
	Use:   "tag <id> <tag1,tag2,...>",
	Short: "Add or remove tags on a session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		shortID := args[0]
		tags := strings.Split(args[1], ",")

		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}

		if tagRemove {
			if err := store.RemoveTags(shortID, tags); err != nil {
				return err
			}
			fmt.Printf("Removed tags from %s\n", shortID)
		} else {
			if err := store.AddTags(shortID, tags); err != nil {
				return err
			}
			fmt.Printf("Added tags to %s\n", shortID)
		}

		// Show current tags
		session, err := store.GetSession(shortID)
		if err != nil {
			return nil
		}
		if len(session.Tags) > 0 {
			fmt.Printf("Tags: %s\n", strings.Join(session.Tags, ", "))
		} else {
			fmt.Println("Tags: (none)")
		}
		return nil
	},
}

func init() {
	tagCmd.Flags().BoolVar(&tagRemove, "remove", false, "remove tags instead of adding")
	rootCmd.AddCommand(tagCmd)
}
