package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timae/ses/internal/gitutil"
)

var (
	diffStat      bool
	diffFilesOnly bool
)

var diffCmd = &cobra.Command{
	Use:   "diff <id>",
	Short: "Show git diff from a session's time window",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := store.GetSession(args[0])
		if err != nil {
			return err
		}

		repoPath := session.Project
		if !gitutil.IsGitRepo(repoPath) {
			return fmt.Errorf("%s is not a git repository", repoPath)
		}

		// Find commits in the session time window
		commits, err := gitutil.CommitsInRange(repoPath, session.StartedAt, session.EndedAt)
		if err != nil {
			return fmt.Errorf("finding commits: %w", err)
		}

		var fromCommit, toCommit string

		if len(commits) >= 2 {
			// commits are newest-first from git log
			toCommit = commits[0]
			fromCommit = commits[len(commits)-1] + "~1" // parent of earliest
		} else if len(commits) == 1 {
			toCommit = commits[0]
			fromCommit = commits[0] + "~1"
		} else if session.GitCommit != "" {
			// Fallback: use stored start commit to HEAD
			fromCommit = session.GitCommit
			toCommit = "HEAD"
		} else {
			return fmt.Errorf("no commits found between %s and %s, and no start commit recorded",
				session.StartedAt.Format("2006-01-02 15:04"),
				session.EndedAt.Format("2006-01-02 15:04"))
		}

		if diffFilesOnly {
			files, err := gitutil.FilesChanged(repoPath, fromCommit, toCommit)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Println(f)
			}
			return nil
		}

		var output string
		if diffStat {
			output, err = gitutil.DiffStat(repoPath, fromCommit, toCommit)
		} else {
			output, err = gitutil.DiffRange(repoPath, fromCommit, toCommit)
		}
		if err != nil {
			return err
		}

		// Pipe through pager if output is large
		if len(output) > 5000 && !diffStat {
			return pagerOutput(output)
		}

		fmt.Print(output)
		return nil
	},
}

func pagerOutput(content string) error {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	cmd := exec.Command(pager)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	diffCmd.Flags().BoolVar(&diffStat, "stat", false, "show file-level summary only")
	diffCmd.Flags().BoolVar(&diffFilesOnly, "files-only", false, "list changed file names only")
	rootCmd.AddCommand(diffCmd)
}
