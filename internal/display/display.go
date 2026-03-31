package display

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/timae/rel.ai/internal/model"
)

var (
	bold   = color.New(color.Bold)
	dim    = color.New(color.Faint)
	cyan   = color.New(color.FgCyan)
	yellow = color.New(color.FgYellow)
)

func SessionTable(sessions []model.Session) {
	if len(sessions) == 0 {
		dim.Println("No sessions found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	bold.Fprintf(w, "ID\tSOURCE\tSTARTED\tPROJECT\tFIRST PROMPT\tTAGS\n")

	for _, s := range sessions {
		prompt := Truncate(s.FirstPrompt, 50)
		project := shortenPath(s.Project)
		tags := strings.Join(s.Tags, ",")
		source := string(s.SourceType)
		id := s.ShortID
		if len(id) > 8 {
			id = id[:8]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			id, source, FormatTime(s.StartedAt), project, prompt, tags)
	}

	w.Flush()
}

func SessionDetail(s *model.Session) {
	bold.Printf("Session %s\n", s.ShortID)
	fmt.Println(strings.Repeat("─", 60))

	field("Source", fmt.Sprintf("%s (%s)", s.SourceType, s.Model))
	field("Session ID", s.SourceID)
	field("Started", s.StartedAt.Format(time.RFC3339))
	if !s.EndedAt.IsZero() {
		field("Ended", s.EndedAt.Format(time.RFC3339))
		field("Duration", s.EndedAt.Sub(s.StartedAt).Round(time.Second).String())
	}
	field("Project", s.Project)
	field("CWD", s.CWD)
	if s.GitBranch != "" {
		field("Git Branch", s.GitBranch)
	}
	if s.GitCommit != "" {
		field("Git Commit", s.GitCommit)
	}
	field("Messages", fmt.Sprintf("%d", s.MessageCount))
	field("Tool Calls", fmt.Sprintf("%d", s.ToolCallCount))

	fmt.Println()
	bold.Println("First Prompt")
	fmt.Println(Truncate(s.FirstPrompt, 500))

	if s.LastAssistant != "" {
		fmt.Println()
		bold.Println("Last Assistant Response")
		fmt.Println(Truncate(s.LastAssistant, 500))
	}

	if len(s.Files) > 0 {
		fmt.Println()
		bold.Println("Files Touched")
		for _, f := range s.Files {
			dim.Printf("  [%s] ", f.Action)
			fmt.Println(f.FilePath)
		}
	}

	if len(s.Tags) > 0 {
		fmt.Println()
		bold.Println("Tags")
		for _, t := range s.Tags {
			yellow.Printf("  #%s", t)
		}
		fmt.Println()
	}
}

func field(label, value string) {
	cyan.Printf("  %-14s", label+":")
	fmt.Printf(" %s\n", value)
}

func FormatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if now.Sub(t) < 7*24*time.Hour {
		return t.Format("Mon 15:04")
	}
	return t.Format("2006-01-02")
}

func Truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func shortenPath(p string) string {
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}
