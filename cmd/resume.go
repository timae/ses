package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/model"
	"github.com/timae/rel.ai/internal/resume"
)

var (
	resumeInject bool
	resumeChain  bool
	resumeTarget string
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Generate a context blob for resuming a session",
	Long:  "Reads the session transcript and generates markdown suitable for pasting into a new AI session.\n\nUsage:\n  ses resume <id> | pbcopy\n  ses resume <id> --inject     Launch a new CLI session with context\n  ses resume <id> --chain      Include linked sessions in context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := store.GetSession(args[0])
		if err != nil {
			return err
		}

		messages, err := loadTranscript(session)
		if err != nil {
			return fmt.Errorf("loading transcript: %w", err)
		}

		var output string

		if resumeChain {
			output, err = generateChainResume(session, messages)
			if err != nil {
				return err
			}
		} else {
			output = resume.Generate(session, messages)
		}

		if resumeInject {
			return injectIntoSession(session, output)
		}

		fmt.Print(output)
		return nil
	},
}

func generateChainResume(session *model.Session, messages []model.Message) (string, error) {
	// Get linked sessions
	linked, err := store.GetLinkedSessions(session.ShortID)
	if err != nil {
		return "", err
	}

	if len(linked) == 0 {
		return resume.Generate(session, messages), nil
	}

	// Build chain context: linked sessions first (chronological), then main session
	var chainParts []string
	for _, ls := range linked {
		lsMessages, err := loadTranscript(&ls.Session)
		if err != nil {
			continue
		}
		part := resume.GenerateBrief(&ls.Session, lsMessages)
		if ls.Reason != "" {
			part = fmt.Sprintf("### Linked session (%s)\n\n%s", ls.Reason, part)
		}
		chainParts = append(chainParts, part)
	}

	mainPart := resume.Generate(session, messages)

	if len(chainParts) > 0 {
		header := "# Session Chain\n\nThis task spans multiple sessions. Here's the full context:\n\n---\n\n"
		chain := ""
		for _, p := range chainParts {
			chain += p + "\n\n---\n\n"
		}
		return header + chain + mainPart, nil
	}

	return mainPart, nil
}

func injectIntoSession(session *model.Session, contextBlob string) error {
	// Write context to temp file
	tmpFile, err := os.CreateTemp("", "ses-resume-*.md")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpFile.WriteString(contextBlob)
	tmpFile.Close()

	target := resumeTarget
	if target == "" {
		target = string(session.SourceType)
	}

	switch target {
	case "claude":
		return launchClaude(session, tmpFile.Name())
	case "codex":
		return launchCodex(session, contextBlob, tmpFile.Name())
	default:
		return fmt.Errorf("unknown target %q (use claude or codex)", target)
	}
}

func launchClaude(session *model.Session, contextFile string) error {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH — install Claude Code first")
	}

	args := []string{
		"claude",
		"--append-system-prompt-file", contextFile,
	}
	if session.Project != "" {
		args = append(args, "--cd", session.Project)
	}

	fmt.Printf("Launching Claude Code in %s with session context...\n", filepath.Base(session.Project))
	return syscall.Exec(claudeBin, args, os.Environ())
}

func launchCodex(session *model.Session, contextBlob, contextFile string) error {
	codexBin, err := exec.LookPath("codex")
	if err != nil {
		return fmt.Errorf("codex not found in PATH — install Codex CLI first")
	}

	// Codex takes a prompt as argument — provide a truncated summary
	prompt := contextBlob
	if len(prompt) > 4000 {
		prompt = prompt[:4000] + "\n\n[Context truncated — see full context in session history]"
	}

	args := []string{"codex"}
	if session.Project != "" {
		args = append(args, "--cd", session.Project)
	}
	args = append(args, prompt)

	fmt.Printf("Launching Codex CLI in %s with session context...\n", filepath.Base(session.Project))
	return syscall.Exec(codexBin, args, os.Environ())
}

func loadTranscript(session *model.Session) ([]model.Message, error) {
	f, err := os.Open(session.TranscriptPath)
	if err != nil {
		return nil, fmt.Errorf("transcript file not found at %s (may have been deleted)", session.TranscriptPath)
	}
	defer f.Close()

	var messages []model.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	switch session.SourceType {
	case model.SourceClaude:
		messages = loadClaudeTranscript(sc)
	case model.SourceCodex:
		messages = loadCodexTranscript(sc)
	}

	return messages, nil
}

func loadClaudeTranscript(sc *bufio.Scanner) []model.Message {
	var messages []model.Message
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}

		switch line.Type {
		case "user":
			text := extractText(line.Message.Content)
			if text != "" {
				messages = append(messages, model.Message{Role: "user", Content: text})
			}
		case "assistant":
			text := extractText(line.Message.Content)
			if text != "" {
				messages = append(messages, model.Message{Role: "assistant", Content: text})
			}
		}
	}
	return messages
}

func loadCodexTranscript(sc *bufio.Scanner) []model.Message {
	var messages []model.Message
	for sc.Scan() {
		var line struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}

		if line.Type == "event_msg" {
			var evt struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}
			if json.Unmarshal(line.Payload, &evt) != nil {
				continue
			}
			switch evt.Type {
			case "user_message":
				if evt.Message != "" {
					messages = append(messages, model.Message{Role: "user", Content: evt.Message})
				}
			case "agent_message":
				if evt.Message != "" {
					messages = append(messages, model.Message{Role: "assistant", Content: evt.Message})
				}
			}
		}
	}
	return messages
}

func extractText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}

func init() {
	resumeCmd.Flags().BoolVar(&resumeInject, "inject", false, "launch a new CLI session with context pre-loaded")
	resumeCmd.Flags().BoolVar(&resumeChain, "chain", false, "include linked sessions in context")
	resumeCmd.Flags().StringVar(&resumeTarget, "target", "", "target CLI (claude|codex), defaults to session source")
	rootCmd.AddCommand(resumeCmd)
}
