package tray

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/timae/rel.ai/internal/db"
	"github.com/timae/rel.ai/internal/display"
)

func Run() {
	go refreshLoop()

	app := menuet.App()
	app.SetMenuState(&menuet.MenuState{
		Title: "🤖",
	})
	app.Label = "ai.rel.ses.menu"
	app.Children = menuItems
	app.RunApplication()
}

func menuItems() []menuet.MenuItem {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".ses", "index.db")

	store, err := db.Open(dbPath)
	if err != nil {
		return []menuet.MenuItem{
			{Text: "Database error", FontSize: 12},
		}
	}
	defer store.Close()

	var items []menuet.MenuItem

	// Daemon status
	if isDaemonRunning() {
		items = append(items, menuet.MenuItem{Text: "Daemon: running", FontSize: 12})
	} else {
		items = append(items, menuet.MenuItem{Text: "Daemon: stopped", FontSize: 12})
	}

	items = append(items, menuet.MenuItem{Type: menuet.Separator})

	// Recent sessions
	items = append(items, menuet.MenuItem{Text: "Recent Sessions", FontSize: 10})

	sessions, err := store.ListSessions(db.ListFilter{Limit: 5})
	if err == nil && len(sessions) > 0 {
		for _, s := range sessions {
			id := s.ShortID
			if len(id) > 8 {
				id = id[:8]
			}
			prompt := display.Truncate(s.FirstPrompt, 35)
			when := display.FormatTime(s.StartedAt)

			capturedID := id
			items = append(items, menuet.MenuItem{
				Text: fmt.Sprintf("%s  %s  %s", id, when, prompt),
				Clicked: func() {
					copyResume(capturedID)
				},
			})
		}
	} else {
		items = append(items, menuet.MenuItem{Text: "  No sessions yet — run ses scan", FontSize: 11})
	}

	items = append(items, menuet.MenuItem{Type: menuet.Separator})

	// Quick stats
	stats, err := store.GetStats(db.ListFilter{})
	if err == nil {
		items = append(items, menuet.MenuItem{
			Text:     fmt.Sprintf("%d sessions (%d this week)", stats.Total, stats.ThisWeek),
			FontSize: 11,
		})
	}

	items = append(items, menuet.MenuItem{Type: menuet.Separator})

	// Actions
	items = append(items, menuet.MenuItem{
		Text: "Scan Now",
		Clicked: func() {
			exec.Command(sesBin(), "scan").Run()
		},
	})
	items = append(items, menuet.MenuItem{
		Text: "Restart Daemon",
		Clicked: func() {
			exec.Command(sesBin(), "watch", "--uninstall").Run()
			exec.Command(sesBin(), "watch", "--install").Run()
		},
	})

	return items
}

func refreshLoop() {
	for {
		time.Sleep(30 * time.Second)
		menuet.App().MenuChanged()
	}
}

func isDaemonRunning() bool {
	err := exec.Command("launchctl", "list", "ai.rel.ses.watch").Run()
	return err == nil
}

func sesBin() string {
	// The menu app lives next to ses in the same directory
	exe, _ := os.Executable()
	candidate := filepath.Join(filepath.Dir(exe), "ses")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// Try common Go install paths
	home, _ := os.UserHomeDir()
	candidate = filepath.Join(home, "go", "bin", "ses")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// Fallback to PATH
	if p, err := exec.LookPath("ses"); err == nil {
		return p
	}
	return "ses"
}

func copyResume(shortID string) {
	command := fmt.Sprintf("ses resume %s", shortID)

	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(command)
	cmd.Run()

	menuet.App().Notification(menuet.Notification{
		Title:   "Copied to clipboard",
		Message: command,
	})
}
