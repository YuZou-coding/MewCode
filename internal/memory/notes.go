package memory

import (
	"os"
	"path/filepath"
)

type Notes struct {
	HomeDir     string
	ProjectRoot string
}

func (n Notes) ReadUser() string {
	return readFile(UserNotesPath(n.HomeDir))
}

func (n Notes) ReadProject() string {
	return readFile(ProjectNotesPath(n.ProjectRoot))
}

func (n Notes) WriteUser(content string) error {
	return writeFile(UserNotesPath(n.HomeDir), content)
}

func (n Notes) WriteProject(content string) error {
	return writeFile(ProjectNotesPath(n.ProjectRoot), content)
}

func (n Notes) Clear(target string) error {
	switch target {
	case "user":
		return n.WriteUser("")
	case "project":
		return n.WriteProject("")
	case "all":
		if err := n.WriteUser(""); err != nil {
			return err
		}
		return n.WriteProject("")
	default:
		return os.ErrInvalid
	}
}

func readFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func writeFile(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}
