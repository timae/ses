package tray

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/timae/rel.ai/internal/db"
	"github.com/timae/rel.ai/internal/display"
)

// Search state is held at the package level because menuet rebuilds the menu
// on demand by calling menuItems(). The click handler that triggers a search
// runs on a different goroutine than the one rendering the menu, so the
// mutex is necessary even though contention is effectively zero.
var (
	searchMu    sync.RWMutex
	searchQuery string
)

const searchResultsLimit = 10

func setSearchQuery(q string) {
	searchMu.Lock()
	searchQuery = q
	searchMu.Unlock()
	menuet.App().MenuChanged()
}

func getSearchQuery() string {
	searchMu.RLock()
	defer searchMu.RUnlock()
	return searchQuery
}

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

	// Active search (if any) takes the top slot because it's the user's
	// current focus; recent sessions fall below.
	if q := getSearchQuery(); q != "" {
		items = append(items, menuet.MenuItem{
			Text:     fmt.Sprintf("Search: %s", display.Truncate(q, 30)),
			FontSize: 10,
		})
		results, err := store.Search(q, searchResultsLimit)
		if err != nil {
			items = append(items, menuet.MenuItem{
				Text: "  search error: " + err.Error(), FontSize: 11,
			})
		} else if len(results) == 0 {
			items = append(items, menuet.MenuItem{Text: "  no matches", FontSize: 11})
		} else {
			for _, s := range results {
				id := s.ShortID
				if len(id) > 8 {
					id = id[:8]
				}
				capturedID := id
				prompt := display.Truncate(s.FirstPrompt, 35)
				when := display.FormatTime(s.StartedAt)
				items = append(items, menuet.MenuItem{
					Text: fmt.Sprintf("%s  %s  %s", id, when, prompt),
					Clicked: func() {
						copyResumeCommand(capturedID)
					},
				})
			}
		}
		items = append(items, menuet.MenuItem{
			Text:    "Clear search",
			Clicked: func() { setSearchQuery("") },
		})
		items = append(items, menuet.MenuItem{Type: menuet.Separator})
	}

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
					copyResumeCommand(capturedID)
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
		Text:    "Search…",
		Clicked: promptSearch,
	})
	items = append(items, menuet.MenuItem{
		Text:    "Open picker in terminal…",
		Clicked: openPickerInTerminal,
	})
	items = append(items, menuet.MenuItem{Type: menuet.Separator})
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

// promptSearch pops a native NSAlert with a single text input. The user
// types an FTS5 query and confirms; the query is stashed in package state
// and the menu re-renders with matches inline.
func promptSearch() {
	pre := getSearchQuery()
	clicked := menuet.App().Alert(menuet.Alert{
		MessageText:     "Search sessions",
		InformativeText: "FTS5 syntax supported. Results appear in the menu after you confirm.",
		Buttons:         []string{"Search", "Cancel"},
		Inputs:          []string{pre},
	})
	if clicked.Button != 0 { // Cancel or dismissed
		return
	}
	var q string
	if len(clicked.Inputs) > 0 {
		q = strings.TrimSpace(clicked.Inputs[0])
	}
	setSearchQuery(q)
}

// openPickerInTerminal launches the bubble-tea picker in a new terminal
// window. iTerm is preferred when installed; Terminal.app is the fallback.
// osascript is the path that actually runs a command on open, not just
// "open -a" which only launches the app.
func openPickerInTerminal() {
	bin := sesBin()
	// Quote the binary path in case it contains spaces.
	cmd := fmt.Sprintf(`%s resume`, shellQuote(bin))
	if hasApp("iTerm") {
		script := fmt.Sprintf(`tell application "iTerm"
	activate
	create window with default profile command %q
end tell`, cmd)
		exec.Command("osascript", "-e", script).Run()
		return
	}
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script %q
end tell`, cmd)
	exec.Command("osascript", "-e", script).Run()
}

func hasApp(name string) bool {
	// `osascript -e 'id of application "Foo"'` exits 0 iff the app is installed.
	err := exec.Command("osascript", "-e", fmt.Sprintf(`id of application %q`, name)).Run()
	return err == nil
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes.
// Safer than hoping exec paths never contain spaces.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func copyResumeCommand(shortID string) {
	command := fmt.Sprintf("ses resume %s --inject", shortID)

	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(command)
	cmd.Run()

	menuet.App().Notification(menuet.Notification{
		Title:   "Copied to clipboard",
		Message: command,
	})
}
