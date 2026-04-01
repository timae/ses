package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/display"
	"github.com/timae/rel.ai/internal/scanner"
)

var watchQuiet bool

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for new sessions and auto-index them",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		claudeDir := filepath.Join(home, ".claude", "projects")
		codexDir := filepath.Join(home, ".codex", "sessions")

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("creating watcher: %w", err)
		}
		defer watcher.Close()

		// Watch directories
		watched := 0
		for _, dir := range []string{claudeDir, codexDir} {
			if err := watchRecursive(watcher, dir); err == nil {
				watched++
			}
		}

		if watched == 0 {
			return fmt.Errorf("no session directories found to watch")
		}

		if !watchQuiet {
			color.New(color.FgCyan).Println("Watching for new sessions... (Ctrl+C to stop)")
		}

		// Debounce timer
		var debounceTimer *time.Timer
		pendingFiles := make(map[string]bool)

		// Signal handling
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		claudeScanner := scanner.NewClaudeScanner(filepath.Join(home, ".claude"))
		codexScanner := scanner.NewCodexScanner(filepath.Join(home, ".codex"))

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}

				// Only care about .jsonl creates/writes
				if !strings.HasSuffix(event.Name, ".jsonl") {
					// But add new directories to watcher
					if event.Has(fsnotify.Create) {
						if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
							watcher.Add(event.Name)
						}
					}
					continue
				}

				if !event.Has(fsnotify.Create) && !event.Has(fsnotify.Write) {
					continue
				}

				pendingFiles[event.Name] = true

				// Debounce: wait 500ms after last event
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
					for path := range pendingFiles {
						processFile(path, claudeScanner, codexScanner)
					}
					pendingFiles = make(map[string]bool)
				})

			case err, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
				if !watchQuiet {
					fmt.Fprintf(os.Stderr, "watch error: %v\n", err)
				}

			case <-sigCh:
				if !watchQuiet {
					fmt.Println("\nStopped watching.")
				}
				return nil
			}
		}
	},
}

func processFile(path string, claudeScanner *scanner.ClaudeScanner, codexScanner *scanner.CodexScanner) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	var sf scanner.SessionFile
	var sc scanner.Scanner

	home, _ := os.UserHomeDir()
	claudeProjects := filepath.Join(home, ".claude", "projects")
	codexSessions := filepath.Join(home, ".codex", "sessions")

	if strings.HasPrefix(path, claudeProjects) {
		sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		sf = scanner.SessionFile{
			TranscriptPath: path,
			SourceType:     "claude",
			SourceID:       sessionID,
			Mtime:          info.ModTime().Unix(),
			Size:           info.Size(),
		}
		sc = claudeScanner
	} else if strings.HasPrefix(path, codexSessions) {
		sessionID := scanner.ExtractCodexSessionID(filepath.Base(path))
		if sessionID == "" {
			return
		}
		sf = scanner.SessionFile{
			TranscriptPath: path,
			SourceType:     "codex",
			SourceID:       sessionID,
			Mtime:          info.ModTime().Unix(),
			Size:           info.Size(),
		}
		sc = codexScanner
	} else {
		return
	}

	session, messages, err := sc.Parse(sf)
	if err != nil {
		return
	}

	if err := store.InsertSession(session, messages); err != nil {
		return
	}

	if !watchQuiet {
		project := display.Truncate(session.Project, 40)
		prompt := display.Truncate(session.FirstPrompt, 50)
		color.New(color.FgGreen).Printf("+ %s ", session.ShortID[:8])
		fmt.Printf("(%s) %s — %s\n", session.SourceType, project, prompt)
	}
}

func watchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
}

func init() {
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "suppress output")
	rootCmd.AddCommand(watchCmd)
}
