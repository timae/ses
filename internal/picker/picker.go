package picker

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/timae/ses/internal/model"
)

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	sourceStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

type Model struct {
	sessions   []model.Session
	filtered   []model.Session
	cursor     int
	search     textinput.Model
	selected   *model.Session
	cancelled  bool
	height     int
}

func New(sessions []model.Session) Model {
	ti := textinput.New()
	ti.Placeholder = "Search sessions..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 60
	ti.Prompt = "🔍 "

	return Model{
		sessions: sessions,
		filtered: sessions,
		search:   ti,
		height:   15,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height - 6 // reserve space for search + header + footer
		if m.height < 3 {
			m.height = 3
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
				s := m.filtered[m.cursor]
				m.selected = &s
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "ctrl+n":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		}
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)

	// Filter on every keystroke
	query := strings.ToLower(m.search.Value())
	if query == "" {
		m.filtered = m.sessions
	} else {
		m.filtered = nil
		for _, s := range m.sessions {
			haystack := strings.ToLower(s.FirstPrompt + " " + s.Project + " " + string(s.SourceType) + " " + strings.Join(s.Tags, " "))
			if strings.Contains(haystack, query) {
				m.filtered = append(m.filtered, s)
			}
		}
	}

	// Keep cursor in bounds
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	return m, cmd
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.search.View())
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d sessions", len(m.filtered))))
	b.WriteString("\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matching sessions.\n"))
		return b.String()
	}

	// Visible window
	start := 0
	if m.cursor >= m.height {
		start = m.cursor - m.height + 1
	}
	end := start + m.height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := start; i < end; i++ {
		s := m.filtered[i]
		id := s.ShortID
		if len(id) > 8 {
			id = id[:8]
		}

		prompt := s.FirstPrompt
		if len(prompt) > 55 {
			prompt = prompt[:52] + "..."
		}
		prompt = strings.ReplaceAll(prompt, "\n", " ")

		source := sourceStyle.Render(fmt.Sprintf("%-6s", s.SourceType))
		when := dimStyle.Render(formatTime(s.StartedAt))

		line := fmt.Sprintf("  %s  %s  %s  %s", id, source, when, prompt)

		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑↓ navigate • enter select • esc cancel"))

	return b.String()
}

func (m Model) Selected() *model.Session {
	return m.selected
}

func (m Model) Cancelled() bool {
	return m.cancelled
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}
