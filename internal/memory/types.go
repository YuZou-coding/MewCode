package memory

import "path/filepath"

const (
	NotesFile          = "notes.md"
	NoteUpdateInterval = 6
)

func UserNotesPath(homeDir string) string {
	return filepath.Join(homeDir, ".mewcode", NotesFile)
}

func ProjectNotesPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".mewcode", NotesFile)
}
