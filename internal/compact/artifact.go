package compact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mewcode/internal/chat"
)

type ArtifactStore struct {
	Root string
	Now  func() time.Time
}

func (s ArtifactStore) WriteToolResult(result chat.ToolResult) (string, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	dir := filepath.Join(s.Root, ArtifactDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s_%s_%s.json", now.UTC().Format("20060102T150405000000000Z"), sanitize(result.Name), sanitize(result.CallID))
	path := filepath.Join(dir, name)
	record := ArtifactRecord{
		ToolName:     result.Name,
		CallID:       result.CallID,
		OriginalSize: ToolResultSize(result),
		CreatedAt:    now.UTC().Format(time.RFC3339Nano),
		Content:      string(result.Content),
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return "", err
	}
	return path, nil
}

var invalidName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func sanitize(value string) string {
	value = strings.Trim(invalidName.ReplaceAllString(value, "_"), "_")
	if value == "" {
		return "tool_result"
	}
	return value
}
