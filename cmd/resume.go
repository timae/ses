package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/timae/rel.ai/internal/model"
	"github.com/timae/rel.ai/internal/resume"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Generate a context blob for resuming a session",
	Long:  "Reads the session transcript and generates markdown suitable for pasting into a new AI session.\nUsage: ses resume <id> | pbcopy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		session, err := store.GetSession(args[0])
		if err != nil {
			return err
		}

		// Re-parse the transcript for full message content
		messages, err := loadTranscript(session)
		if err != nil {
			return fmt.Errorf("loading transcript: %w", err)
		}

		output := resume.Generate(session, messages)
		fmt.Print(output)
		return nil
	},
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
	rootCmd.AddCommand(resumeCmd)
}
