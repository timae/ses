package model

import "time"

type Source string

const (
	SourceClaude Source = "claude"
	SourceCodex  Source = "codex"
)

type Session struct {
	ID              int64
	ShortID         string
	SourceType      Source
	SourceID        string
	PID             int
	Project         string
	CWD             string
	GitBranch       string
	GitCommit       string
	StartedAt       time.Time
	EndedAt         time.Time
	MessageCount    int
	ToolCallCount   int
	FirstPrompt     string
	LastAssistant   string
	Model           string
	TranscriptPath  string
	ScannedAt       time.Time
	TranscriptMtime time.Time
	TranscriptSize  int64
	Tags            []string
	Files           []SessionFile
	UserPrompts     []string
}

type SessionFile struct {
	FilePath string
	Action   string // "read", "write", "edit"
}

type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	ToolName  string
	FilePath  string
}
