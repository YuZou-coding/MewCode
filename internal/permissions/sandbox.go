package permissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func CheckSandbox(request Request) (Decision, bool) {
	path, ok := representativePath(request)
	if !ok || path == "" {
		return Decision{}, false
	}
	root := request.Root
	if root == "" {
		root = "."
	}
	inside, err := insideRoot(root, path)
	if err != nil {
		return Deny("path_outside_sandbox", err.Error()), true
	}
	if !inside {
		return Deny("path_outside_sandbox", "path is outside project sandbox"), true
	}
	return Decision{}, false
}

func representativePath(request Request) (string, bool) {
	switch request.Tool {
	case "read_file", "write_file", "edit_file":
		var args struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(request.Arguments, &args)
		return args.Path, args.Path != ""
	case "find_files":
		var args struct {
			Pattern string `json:"pattern"`
		}
		_ = json.Unmarshal(request.Arguments, &args)
		return args.Pattern, args.Pattern != ""
	case "search_code":
		var args struct {
			Root string `json:"root"`
		}
		_ = json.Unmarshal(request.Arguments, &args)
		if args.Root == "" {
			return ".", true
		}
		return args.Root, true
	default:
		return "", false
	}
}

func insideRoot(root string, candidate string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	if rootEval, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = rootEval
	}
	candidatePath := candidate
	if strings.ContainsAny(candidate, "*?[") {
		candidatePath = globBase(candidate)
	}
	if candidatePath == "" {
		candidatePath = "."
	}
	if !filepath.IsAbs(candidatePath) {
		candidatePath = filepath.Join(rootAbs, candidatePath)
	}
	candidateAbs, err := filepath.Abs(candidatePath)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(candidateAbs); err == nil {
		if eval, evalErr := filepath.EvalSymlinks(candidateAbs); evalErr == nil {
			candidateAbs = eval
		}
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."), nil
}

func globBase(pattern string) string {
	clean := filepath.Clean(pattern)
	parts := strings.Split(clean, string(filepath.Separator))
	var kept []string
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return "."
	}
	return filepath.Join(kept...)
}
