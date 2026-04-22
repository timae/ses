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

type ClaudeScanner struct {
	Home        string // ~/.claude
	sessionMeta map[string]claudeSessionMeta
	historyMap  map[string]claudeHistoryEntry
}

func NewClaudeScanner(home string) *ClaudeScanner {
	return &ClaudeScanner{
		Home:        home,
		sessionMeta: make(map[string]claudeSessionMeta),
		historyMap:  make(map[string]claudeHistoryEntry),
	}
}

// claudeSessionMeta from sessions/{PID}.json
type claudeSessionMeta struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"` // ms epoch
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// claudeHistoryEntry from history.jsonl
type claudeHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // ms epoch
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// claudeTranscriptLine from projects/*/sessionId.jsonl
type claudeTranscriptLine struct {
	Type      string             `json:"type"` // "user", "assistant", "file-history-snapshot", "progress"
	UUID      string             `json:"uuid"`
	SessionID string             `json:"sessionId"`
	Timestamp string             `json:"timestamp"`
	CWD       string             `json:"cwd"`
	GitBranch string             `json:"gitBranch"`
	Version   string             `json:"version"`
	Message   claudeMessage      `json:"message"`
}

type claudeMessage struct {
	Role    string            `json:"role"`
	Model   string            `json:"model"`
	Content json.RawMessage   `json:"content"`
}

type claudeContentBlock struct {
	Type  string          `json:"type"` // "text", "tool_use", "tool_result", "thinking"
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (s *ClaudeScanner) Discover() ([]SessionFile, error) {
	// Build session metadata map from sessions/*.json
	sessionMeta := make(map[string]claudeSessionMeta)
	metaFiles, _ := filepath.Glob(filepath.Join(s.Home, "sessions", "*.json"))
	for _, f := range metaFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m claudeSessionMeta
		if json.Unmarshal(data, &m) == nil && m.SessionID != "" {
			sessionMeta[m.SessionID] = m
		}
	}

	// Build history map
	historyMap := make(map[string]claudeHistoryEntry)
	histFile := filepath.Join(s.Home, "history.jsonl")
	if f, err := os.Open(histFile); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var e claudeHistoryEntry
			if json.Unmarshal(sc.Bytes(), &e) == nil && e.SessionID != "" {
				historyMap[e.SessionID] = e
			}
		}
		f.Close()
	}

	// Find transcript files
	projectDirs, _ := filepath.Glob(filepath.Join(s.Home, "projects", "*"))
	var files []SessionFile

	for _, dir := range projectDirs {
		transcripts, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		for _, tp := range transcripts {
			sessionID := strings.TrimSuffix(filepath.Base(tp), ".jsonl")
			info, err := os.Stat(tp)
			if err != nil {
				continue
			}

			files = append(files, SessionFile{
				TranscriptPath: tp,
				SourceType:     model.SourceClaude,
				SourceID:       sessionID,
				Mtime:          info.ModTime().Unix(),
				Size:           info.Size(),
			})
		}
	}

	// Store maps for use during Parse
	s.sessionMeta = sessionMeta
	s.historyMap = historyMap

	return files, nil
}

var _ Scanner = (*ClaudeScanner)(nil)

func (s *ClaudeScanner) Parse(sf SessionFile) (*model.Session, []model.Message, error) {

	f, err := os.Open(sf.TranscriptPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	session := &model.Session{
		ShortID:         ShortID(model.SourceClaude, sf.SourceID),
		SourceType:      model.SourceClaude,
		SourceID:        sf.SourceID,
		TranscriptPath:  sf.TranscriptPath,
		TranscriptMtime: time.Unix(sf.Mtime, 0),
		TranscriptSize:  sf.Size,
	}

	// Apply metadata from sessions/*.json
	if meta, ok := s.sessionMeta[sf.SourceID]; ok {
		session.PID = meta.PID
		session.CWD = meta.CWD
		session.StartedAt = time.UnixMilli(meta.StartedAt)
	}

	// Apply project path from history
	if hist, ok := s.historyMap[sf.SourceID]; ok {
		session.Project = hist.Project
	}

	// If no project from history, derive from directory name
	if session.Project == "" {
		dirName := filepath.Base(filepath.Dir(sf.TranscriptPath))
		session.Project = decodeProjectPath(dirName)
	}

	var messages []model.Message
	filesMap := make(map[string]string) // path -> action

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	for sc.Scan() {
		var line claudeTranscriptLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}

		switch line.Type {
		case "user":
			// Extract user text content
			text := extractClaudeText(line.Message.Content)
			if text != "" {
				session.UserPrompts = append(session.UserPrompts, text)
				session.MessageCount++
				messages = append(messages, model.Message{
					Role:    "user",
					Content: text,
				})
				if session.FirstPrompt == "" {
					session.FirstPrompt = text
				}
			}

			// Capture metadata from first user message
			if line.CWD != "" && session.CWD == "" {
				session.CWD = line.CWD
			}
			if line.GitBranch != "" {
				session.GitBranch = line.GitBranch
			}
			if line.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
					if session.StartedAt.IsZero() {
						session.StartedAt = t
					}
					session.EndedAt = t
				}
			}

		case "assistant":
			session.MessageCount++
			text, toolCalls := extractClaudeAssistantContent(line.Message.Content)

			if text != "" {
				session.LastAssistant = text
				messages = append(messages, model.Message{
					Role:    "assistant",
					Content: text,
				})
			}

			// Track model (skip synthetic/internal markers)
			if line.Message.Model != "" && !strings.HasPrefix(line.Message.Model, "<") {
				session.Model = line.Message.Model
			}

			// Process tool calls
			for _, tc := range toolCalls {
				session.ToolCallCount++
				messages = append(messages, model.Message{
					Role:     "tool_use",
					ToolName: tc.Name,
					FilePath: tc.FilePath,
				})
				if tc.FilePath != "" {
					action := "read"
					switch tc.Name {
					case "Write":
						action = "write"
					case "Edit":
						action = "edit"
					}
					filesMap[tc.FilePath] = action
				}
			}
		}
	}

	// Populate files
	for path, action := range filesMap {
		session.Files = append(session.Files, model.SessionFile{
			FilePath: path,
			Action:   action,
		})
	}

	if session.CWD == "" {
		session.CWD = session.Project
	}

	return session, messages, nil
}

type toolCallInfo struct {
	Name     string
	FilePath string
}

func extractClaudeText(raw json.RawMessage) string {
	// Content can be a string or array of content blocks
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}

	var blocks []claudeContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				return b.Text
			}
		}
	}
	return ""
}

func extractClaudeAssistantContent(raw json.RawMessage) (text string, tools []toolCallInfo) {
	var blocks []claudeContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				text = b.Text
			}
		case "tool_use":
			tc := toolCallInfo{Name: b.Name}
			// Try to extract file_path from input
			var input map[string]any
			if json.Unmarshal(b.Input, &input) == nil {
				if fp, ok := input["file_path"].(string); ok {
					tc.FilePath = fp
				}
			}
			tools = append(tools, tc)
		}
	}
	return text, tools
}

func decodeProjectPath(dirName string) string {
	// -Users-tim-Documents-Claude-foo -> /Users/tim/Documents/Claude/foo
	if !strings.HasPrefix(dirName, "-") {
		return dirName
	}
	return "/" + strings.ReplaceAll(dirName[1:], "-", "/")
}
