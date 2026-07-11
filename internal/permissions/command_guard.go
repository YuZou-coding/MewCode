package permissions

import (
	"encoding/json"
	"regexp"
	"strings"
)

var remoteScriptPattern = regexp.MustCompile(`(?i)\b(curl|wget)\b.*\|\s*(sh|bash)\b`)

func CheckDangerousCommand(request Request) (Decision, bool) {
	if request.Tool != "run_command" {
		return Decision{}, false
	}
	command := commandFromArgs(request.Arguments)
	normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
	if normalized == "" {
		return Decision{}, false
	}
	if strings.Contains(normalized, "rm -rf /") || strings.Contains(normalized, "rm -fr /") {
		return Deny("dangerous_command", "dangerous command blocked"), true
	}
	if strings.Contains(normalized, "mkfs") {
		return Deny("dangerous_command", "dangerous command blocked"), true
	}
	if remoteScriptPattern.MatchString(command) {
		return Deny("dangerous_command", "remote script execution blocked"), true
	}
	return Decision{}, false
}

func commandFromArgs(raw json.RawMessage) string {
	var args struct {
		Command string `json:"command"`
	}
	_ = json.Unmarshal(raw, &args)
	return args.Command
}
