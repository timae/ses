package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/scanner"
)

var scanFull bool
var claudeHome string
var codexHome string

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Import sessions from Claude Code and Codex CLI",
	RunE: func(cmd *cobra.Command, args []string) error {
		var scanners []scanner.Scanner

		if _, err := os.Stat(claudeHome); err == nil {
			scanners = append(scanners, scanner.NewClaudeScanner(claudeHome))
		}
		if _, err := os.Stat(codexHome); err == nil {
			scanners = append(scanners, scanner.NewCodexScanner(codexHome))
		}

		if len(scanners) == 0 {
			return fmt.Errorf("no session sources found (checked %s, %s)", claudeHome, codexHome)
		}

		orch := scanner.NewOrchestrator(store, scanners...)
		newCount, updatedCount, err := orch.Scan(scanFull)
		if err != nil {
			return err
		}

		fmt.Printf("Scan complete: %d new, %d updated\n", newCount, updatedCount)
		return nil
	},
}

func init() {
	home, _ := os.UserHomeDir()
	scanCmd.Flags().BoolVar(&scanFull, "full", false, "re-import all sessions (ignore cache)")
	scanCmd.Flags().StringVar(&claudeHome, "claude-home", filepath.Join(home, ".claude"), "Claude Code data directory")
	scanCmd.Flags().StringVar(&codexHome, "codex-home", filepath.Join(home, ".codex"), "Codex CLI data directory")
	rootCmd.AddCommand(scanCmd)
}
