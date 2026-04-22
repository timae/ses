package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/timae/ses/internal/model"
	"github.com/timae/ses/internal/resume"
	"github.com/timae/ses/internal/shareserver"
)

// runResumeFromURL claims a handoff and launches Claude Code (or Codex) in
// the caller's chosen project directory. Always consumes + launches — the
// claim page is the place to "just look" without burning the single-use URL.
func runResumeFromURL(raw, projectOverride string) error {
	base, id, err := parseShareURL(raw)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "claiming handoff %s …\n", id)
	share, err := consumeHandoff(base, id)
	if err != nil {
		return err
	}

	// Save the raw claim locally so the teammate can reference specific turns
	// later. Best-effort — a save failure shouldn't block the launch.
	savedPath, saveErr := saveHandoff(share)
	if saveErr != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't save local copy: %v\n", saveErr)
	}

	project, err := resolveProject(projectOverride)
	if err != nil {
		return err
	}

	// Build a synthetic model.Session from the handoff so the existing inject
	// machinery (launchClaude / launchCodex) works unchanged. The project
	// path is the teammate's local checkout, not the sender's.
	synthSession := &model.Session{
		ShortID:       share.Session.ShortID,
		Project:       project,
		SourceType:    model.Source(share.Session.Source),
		Model:         share.Session.Model,
		GitBranch:     share.Session.GitBranch,
		StartedAt:     share.Session.StartedAt,
		EndedAt:       share.Session.EndedAt,
		MessageCount:  share.Session.MessageCount,
		ToolCallCount: share.Session.ToolCalls,
		FirstPrompt:   share.Session.FirstPrompt,
	}
	if synthSession.SourceType != model.SourceClaude && synthSession.SourceType != model.SourceCodex {
		synthSession.SourceType = model.SourceClaude
	}

	messages := toModelMessages(share.Messages)
	blob := buildHandoffContextBlob(share, messages, savedPath)

	fmt.Fprintf(os.Stderr, "launching %s in %s …\n", synthSession.SourceType, project)
	return injectIntoSession(synthSession, blob)
}

// parseShareURL extracts (baseURL, id) from "https://host/s/{id}" or
// "https://host/s/{id}/anything".
func parseShareURL(raw string) (string, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid --from URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("--from URL must be absolute")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "s" || parts[1] == "" {
		return "", "", fmt.Errorf("--from URL doesn't look like a share URL (expected /s/<id>)")
	}
	base := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	return base, parts[1], nil
}

func consumeHandoff(base, id string) (*shareserver.Share, error) {
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/shares/"+id+"/consume", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("consume request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("handoff not found — the link may have already been claimed")
	}
	if resp.StatusCode == http.StatusGone {
		return nil, fmt.Errorf("handoff expired")
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("consume failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var s shareserver.Share
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("decoding share: %w", err)
	}
	return &s, nil
}

func saveHandoff(s *shareserver.Share) (string, error) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ses", "handoffs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, s.ID+".json")
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func resolveProject(override string) (string, error) {
	target := override
	if target == "" {
		target = "."
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolving --project %q: %w", target, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("--project %q: %w", target, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("--project %q is not a directory", target)
	}
	return abs, nil
}

func toModelMessages(in []shareserver.ShareMsg) []model.Message {
	out := make([]model.Message, len(in))
	for i, m := range in {
		out[i] = model.Message{
			Role:     m.Role,
			Content:  m.Content,
			ToolName: m.ToolName,
			FilePath: m.FilePath,
		}
	}
	return out
}

// buildHandoffContextBlob produces the markdown that gets injected into the
// teammate's Claude Code. Structure: sender's note first (highest priority),
// then prior-session chain, then the main transcript summary, then a pointer
// to the saved JSON for drilling in.
func buildHandoffContextBlob(s *shareserver.Share, messages []model.Message, savedPath string) string {
	var b strings.Builder

	b.WriteString("# Session Handoff\n\n")
	b.WriteString("You're picking up a session someone else was driving. Read the handoff note below first, then the context, then continue the task.\n\n")

	if s.HandoffNote != "" {
		b.WriteString("## Handoff note\n\n")
		b.WriteString(strings.TrimSpace(s.HandoffNote))
		b.WriteString("\n\n")
	}

	b.WriteString("## Original session\n")
	if s.Session.Project != "" {
		fmt.Fprintf(&b, "- **Project (on sender's machine)**: %s\n", s.Session.Project)
	}
	if s.Session.GitBranch != "" {
		fmt.Fprintf(&b, "- **Git branch**: %s\n", s.Session.GitBranch)
	}
	if !s.Session.StartedAt.IsZero() {
		fmt.Fprintf(&b, "- **Started**: %s\n", s.Session.StartedAt.UTC().Format(time.RFC3339))
	}
	if s.Session.Model != "" {
		fmt.Fprintf(&b, "- **Model**: %s\n", s.Session.Model)
	}
	fmt.Fprintf(&b, "- **Messages**: %d\n", s.Session.MessageCount)
	b.WriteString("\n")

	if len(s.Files) > 0 {
		b.WriteString("## Files touched in the original session\n")
		for _, f := range s.Files {
			fmt.Fprintf(&b, "- [%s] %s\n", f.Action, f.Path)
		}
		b.WriteString("\n")
	}

	if len(s.LinkedSessions) > 0 {
		b.WriteString("## Prior sessions in this task\n\n")
		for _, ls := range s.LinkedSessions {
			if ls.Reason != "" {
				fmt.Fprintf(&b, "### Linked (%s)\n\n", ls.Reason)
			}
			b.WriteString(ls.Brief)
			b.WriteString("\n\n---\n\n")
		}
	}

	b.WriteString("## Transcript summary\n\n")
	// Synthesize a session-ish shape for resume.Generate.
	synth := &model.Session{
		ShortID:       s.Session.ShortID,
		SourceType:    model.Source(s.Session.Source),
		Project:       s.Session.Project,
		GitBranch:     s.Session.GitBranch,
		StartedAt:     s.Session.StartedAt,
		EndedAt:       s.Session.EndedAt,
		MessageCount:  s.Session.MessageCount,
		ToolCallCount: s.Session.ToolCalls,
		FirstPrompt:   s.Session.FirstPrompt,
		Model:         s.Session.Model,
	}
	b.WriteString(resume.Generate(synth, messages))

	if savedPath != "" {
		b.WriteString("\n\n---\n")
		fmt.Fprintf(&b, "Full scrubbed transcript saved at `%s` if you need to reference specific turns.\n", savedPath)
	}

	return b.String()
}
