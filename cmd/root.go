package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/timae/ses/internal/db"
)

var (
	dbPath string
	store  *db.DB
)

var rootCmd = &cobra.Command{
	Use:   "ses",
	Short: "AI session manager — capture and resume Claude Code & Codex sessions",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip DB for help commands
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}
		var err error
		store, err = db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if store != nil {
			store.Close()
		}
	},
}

func init() {
	home, _ := os.UserHomeDir()
	defaultDB := filepath.Join(home, ".ses", "index.db")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "path to SQLite database")
}

func Execute() error {
	return rootCmd.Execute()
}
