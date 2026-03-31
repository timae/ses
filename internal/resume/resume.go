package resume

import (
	"fmt"
	"strings"
	"time"

	"github.com/timae/rel.ai/internal/model"
)

func Generate(session *model.Session, messages []model.Message) string {
	var b strings.Builder

	// Header
	title := session.FirstPrompt
	if len(title) > 80 {
		title = title[:77] + "..."
	}
	fmt.Fprintf(&b, "# Session Resume: %s\n\n", title)

	// Context
	b.WriteString("## Context\n")
	fmt.Fprintf(&b, "- **Project**: %s\n", session.Project)
	if session.CWD != "" && session.CWD != session.Project {
		fmt.Fprintf(&b, "- **Working directory**: %s\n", session.CWD)
	}
	if session.GitBranch != "" {
		branch := session.GitBranch
		if session.GitCommit != "" {
			branch += fmt.Sprintf(" (at %s)", session.GitCommit[:min(8, len(session.GitCommit))])
		}
		fmt.Fprintf(&b, "- **Git branch**: %s\n", branch)
	}
	started := session.StartedAt.Format(time.RFC3339)
	if !session.EndedAt.IsZero() {
		duration := session.EndedAt.Sub(session.StartedAt).Round(time.Second)
		fmt.Fprintf(&b, "- **When**: %s (%s)\n", started, duration)
	} else {
		fmt.Fprintf(&b, "- **When**: %s\n", started)
	}
	fmt.Fprintf(&b, "- **Source**: %s", session.SourceType)
	if session.Model != "" {
		fmt.Fprintf(&b, " (%s)", session.Model)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "- **Messages**: %d, **Tool calls**: %d\n", session.MessageCount, session.ToolCallCount)

	// Original goal
	b.WriteString("\n## Original Goal\n")
	fmt.Fprintf(&b, "%s\n", session.FirstPrompt)

	// Key prompts
	if len(session.UserPrompts) > 1 {
		b.WriteString("\n## Key Prompts During Session\n")
		for i, p := range session.UserPrompts {
			if i == 0 {
				continue // skip first prompt, already shown
			}
			// Skip noise: very short, system commands, XML tags
			if len(p) < 10 || isSystemNoise(p) {
				continue
			}
			text := p
			if len(text) > 200 {
				text = text[:197] + "..."
			}
			fmt.Fprintf(&b, "%d. %s\n", i, text)
		}
	}

	// What was accomplished — last substantive assistant messages
	accomplishments := extractAccomplishments(messages)
	if len(accomplishments) > 0 {
		b.WriteString("\n## What Was Accomplished\n")
		for _, a := range accomplishments {
			text := a
			if len(text) > 500 {
				text = text[:497] + "..."
			}
			fmt.Fprintf(&b, "%s\n\n", text)
		}
	}

	// Where it left off
	if session.LastAssistant != "" {
		b.WriteString("\n## Where It Left Off\n")
		text := session.LastAssistant
		if len(text) > 800 {
			text = text[:797] + "..."
		}
		fmt.Fprintf(&b, "%s\n", text)
	}

	// Files touched
	if len(session.Files) > 0 {
		b.WriteString("\n## Files Touched\n")
		for _, f := range session.Files {
			fmt.Fprintf(&b, "- [%s] %s\n", f.Action, f.FilePath)
		}
	}

	// Resume instructions
	b.WriteString("\n## Resume Instructions\n")
	b.WriteString("Continue working on this task. The session was interrupted.\n")
	b.WriteString("Pick up where the previous assistant left off.\n")
	b.WriteString("Review the files listed above for current state.\n")

	return b.String()
}

func extractAccomplishments(messages []model.Message) []string {
	// Walk messages in reverse, collect assistant text messages > 100 chars
	var candidates []string
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" && len(m.Content) > 100 {
			candidates = append(candidates, m.Content)
			if len(candidates) >= 3 {
				break
			}
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(candidates)-1; i < j; i, j = i+1, j-1 {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}

	return candidates
}

func isSystemNoise(p string) bool {
	prefixes := []string{
		"<local-command",
		"<command-name>",
		"<command-message>",
		"[Request interrupted",
		"<system-reminder>",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
