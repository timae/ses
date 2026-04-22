package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/timae/ses/internal/model"
)

type CodexScanner struct {
	Home       string // ~/.codex
	historyMap map[string]codexHistoryEntry
}

func NewCodexScanner(home string) *CodexScanner {
	return &CodexScanner{
		Home:       home,
		historyMap: make(map[string]codexHistoryEntry),
	}
}

var _ Scanner = (*CodexScanner)(nil)

type codexHistoryEntry struct {
	SessionID string `json:"session_id"`
	Ts        int64  `json:"ts"`
	Text      string `json:"text"`
}

type codexTranscriptLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"` // "session_meta", "event_msg", "response_item", "turn_context"
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Git       struct {
		Branch     string `json:"branch"`
		CommitHash string `json:"commit_hash"`
	} `json:"git"`
	ModelProvider string `json:"model_provider"`
	CLIVersion    string `json:"cli_version"`
}

type codexEventPayload struct {
	Type    string `json:"type"` // "task_started", "user_message", "agent_message", "token_count", "task_complete"
	Message string `json:"message"`
	Phase   string `json:"phase"`
}

type codexResponseItem struct {
	Type      string `json:"type"` // "message", "function_call", "function_call_output", "reasoning"
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Role      string `json:"role"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func (s *CodexScanner) Discover() ([]SessionFile, error) {
	// Read history
	histFile := filepath.Join(s.Home, "history.jsonl")
	if f, err := os.Open(histFile); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var e codexHistoryEntry
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.SessionID != "" {
				s.historyMap[e.SessionID] = e
			}
		}
		f.Close()
	}

	// Find transcript files
	pattern := filepath.Join(s.Home, "sessions", "*", "*", "*", "*.jsonl")
	transcripts, _ := filepath.Glob(pattern)

	var files []SessionFile
	for _, tp := range transcripts {
		base := filepath.Base(tp)
		// rollout-{timestamp}-{sessionId}.jsonl
		sessionID := ExtractCodexSessionID(base)
		if sessionID == "" {
			continue
		}

		info, err := os.Stat(tp)
		if err != nil {
			continue
		}

		files = append(files, SessionFile{
			TranscriptPath: tp,
			SourceType:     model.SourceCodex,
			SourceID:       sessionID,
			Mtime:          info.ModTime().Unix(),
			Size:           info.Size(),
		})
	}

	return files, nil
}

func (s *CodexScanner) Parse(sf SessionFile) (*model.Session, []model.Message, error) {
	f, err := os.Open(sf.TranscriptPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	session := &model.Session{
		ShortID:         ShortID(model.SourceCodex, sf.SourceID),
		SourceType:      model.SourceCodex,
		SourceID:        sf.SourceID,
		TranscriptPath:  sf.TranscriptPath,
		TranscriptMtime: time.Unix(sf.Mtime, 0),
		TranscriptSize:  sf.Size,
	}

	// Apply first prompt from history
	if hist, ok := s.historyMap[sf.SourceID]; ok {
		session.FirstPrompt = hist.Text
		session.StartedAt = time.Unix(hist.Ts, 0)
	}

	var messages []model.Message
	filesMap := make(map[string]string)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	for sc.Scan() {
		var line codexTranscriptLine
		if json.Unmarshal(sc.Bytes(), &line) != nil {
			continue
		}

		// Track timestamp for EndedAt
		if line.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				if session.StartedAt.IsZero() {
					session.StartedAt = t
				}
				session.EndedAt = t
			}
		}

		switch line.Type {
		case "session_meta":
			var meta codexSessionMeta
			if json.Unmarshal(line.Payload, &meta) == nil {
				session.CWD = meta.CWD
				session.Project = meta.CWD
				session.GitBranch = meta.Git.Branch
				session.GitCommit = meta.Git.CommitHash
				session.Model = meta.ModelProvider
				if meta.Timestamp != "" {
					if t, err := time.Parse(time.RFC3339Nano, meta.Timestamp); err == nil {
						session.StartedAt = t
					}
				}
			}

		case "event_msg":
			var evt codexEventPayload
			if json.Unmarshal(line.Payload, &evt) != nil {
				continue
			}
			switch evt.Type {
			case "user_message":
				if evt.Message != "" {
					session.MessageCount++
					session.UserPrompts = append(session.UserPrompts, evt.Message)
					messages = append(messages, model.Message{
						Role:    "user",
						Content: evt.Message,
					})
					if session.FirstPrompt == "" {
						session.FirstPrompt = evt.Message
					}
				}
			case "agent_message":
				if evt.Message != "" {
					session.MessageCount++
					session.LastAssistant = evt.Message
					messages = append(messages, model.Message{
						Role:    "assistant",
						Content: evt.Message,
					})
				}
			}

		case "response_item":
			var item codexResponseItem
			if json.Unmarshal(line.Payload, &item) != nil {
				continue
			}
			if item.Type == "function_call" {
				session.ToolCallCount++
				tc := model.Message{
					Role:     "tool_use",
					ToolName: item.Name,
				}
				// Try to extract file paths from arguments
				if item.Name == "exec_command" {
					var args struct {
						Cmd string `json:"cmd"`
					}
					if json.Unmarshal([]byte(item.Arguments), &args) == nil {
						tc.Content = args.Cmd
					}
				}
				messages = append(messages, tc)
			}
		}
	}

	if session.CWD == "" {
		session.CWD = session.Project
	}

	for path, action := range filesMap {
		session.Files = append(session.Files, model.SessionFile{
			FilePath: path,
			Action:   action,
		})
	}

	return session, messages, nil
}

func ExtractCodexSessionID(filename string) string {
	// rollout-2026-03-21T14:50:05.240Z-019d10df-c30d-7f41-8c2e-fb61ecc0271d.jsonl
	name := strings.TrimSuffix(filename, ".jsonl")
	if !strings.HasPrefix(name, "rollout-") {
		return ""
	}
	// Find the UUID: 8-4-4-4-12 pattern after the ISO timestamp
	// Format: rollout-{ISO}-{UUID}
	// The UUID starts after the ISO timestamp which contains 'Z-'
	parts := strings.SplitN(name, "Z-", 2)
	if len(parts) < 2 {
		// Try without Z
		parts = strings.SplitN(name, "-", 2)
		if len(parts) < 2 {
			return ""
		}
		return parts[1]
	}
	return parts[1]
}
