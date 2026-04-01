package gitutil

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func CommitsInRange(repoPath string, since, until time.Time) ([]string, error) {
	args := []string{
		"-C", repoPath,
		"log", "--format=%H",
		"--after=" + since.Format(time.RFC3339),
	}
	if !until.IsZero() {
		args = append(args, "--before="+until.Format(time.RFC3339))
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

func DiffRange(repoPath, fromCommit, toCommit string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "diff", fromCommit+".."+toCommit).Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}

func DiffStat(repoPath, fromCommit, toCommit string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "diff", "--stat", fromCommit+".."+toCommit).Output()
	if err != nil {
		return "", fmt.Errorf("git diff --stat: %w", err)
	}
	return string(out), nil
}

func FilesChanged(repoPath, fromCommit, toCommit string) ([]string, error) {
	out, err := exec.Command("git", "-C", repoPath, "diff", "--name-only", fromCommit+".."+toCommit).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only: %w", err)
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

func IsGitRepo(path string) bool {
	err := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree").Run()
	return err == nil
}
