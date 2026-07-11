package compact

import "mewcode/internal/chat"

const (
	SingleToolResultThreshold = 12000
	MessageToolResultsLimit   = 24000
	HistorySummaryThreshold   = 80000
	ToolResultPreviewLength   = 2000
	RecentRoundsToKeep        = 6
	SummaryFailureLimit       = 3
	ArtifactDir               = ".mewcode/artifacts/tool-results"
	ManualCommand             = "/compact"
)

type Stats struct {
	BeforeMessages int
	AfterMessages  int
	BeforeChars    int
	AfterChars     int
	Artifacts      int
	Summarized     bool
	Errors         []error
}

type Result struct {
	Messages []chat.Message
	Stats    Stats
}

type ArtifactRecord struct {
	ToolName     string `json:"tool_name"`
	CallID       string `json:"call_id"`
	OriginalSize int    `json:"original_size"`
	CreatedAt    string `json:"created_at"`
	Content      string `json:"content"`
}
