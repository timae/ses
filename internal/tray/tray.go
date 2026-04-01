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
			exec.Command("ses", "scan").Run()
		},
	})
	items = append(items, menuet.MenuItem{
		Text: "Restart Daemon",
		Clicked: func() {
			exec.Command("ses", "watch", "--uninstall").Run()
			exec.Command("ses", "watch", "--install").Run()
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

func copyResume(shortID string) {
	out, err := exec.Command("ses", "resume", shortID).Output()
	if err != nil {
		menuet.App().Notification(menuet.Notification{
			Title:   "Resume failed",
			Message: fmt.Sprintf("Could not generate resume for %s", shortID),
		})
		return
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(string(out))
	cmd.Run()

	menuet.App().Notification(menuet.Notification{
		Title:   "Resume copied to clipboard",
		Message: fmt.Sprintf("Session %s — paste into a new Claude/Codex session", shortID),
	})
}
