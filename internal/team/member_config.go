package team

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MemberEntryFile = "MEMBER.md"

func (m *Manager) ReloadMembersFromConfig(teamName string) (int, error) {
	team, err := m.Load(teamName)
	if err != nil {
		return 0, err
	}
	current, err := m.Members(teamName)
	if err != nil {
		return 0, err
	}
	byName := map[string]Member{}
	for _, member := range current {
		byName[strings.ToLower(member.Name)] = member
	}
	configs, warnings, err := loadMemberConfigFiles(filepath.Join(team.Root, "members"))
	for _, warning := range warnings {
		m.addWarning(warning)
	}
	if err != nil {
		return 0, err
	}
	for _, member := range configs {
		if member.Name == "" {
			continue
		}
		key := strings.ToLower(member.Name)
		existing := byName[key]
		if existing.InstanceID != "" && member.InstanceID == "" {
			member.InstanceID = existing.InstanceID
		}
		if member.InstanceID == "" {
			member.InstanceID = member.Name
		}
		if member.Role == "" {
			member.Role = existing.Role
		}
		if member.Role == "" {
			member.Role = "general"
		}
		if member.Workdir == "" {
			member.Workdir = existing.Workdir
		}
		if member.Workdir == "" {
			member.Workdir = m.ProjectRoot
		}
		if member.Backend == "" {
			member.Backend = existing.Backend
		}
		if member.Backend == "" {
			member.Backend = m.Options.DefaultBackend
		}
		if existing.Status != "" {
			member.Status = existing.Status
		}
		if member.Status == "" {
			member.Status = MemberStatusIdle
		}
		if !existing.LastActiveAt.IsZero() {
			member.LastActiveAt = existing.LastActiveAt
		}
		if member.LastActiveAt.IsZero() {
			member.LastActiveAt = time.Now()
		}
		if member.ResumeRef == "" {
			member.ResumeRef = existing.ResumeRef
		}
		if member.ResumeRef == "" {
			member.ResumeRef = filepath.Join(teamName, member.InstanceID)
		}
		byName[key] = member
	}
	merged := make([]Member, 0, len(byName))
	for _, member := range byName {
		merged = append(merged, member)
	}
	return len(configs), m.SaveMembers(teamName, merged)
}

func loadMemberConfigFiles(root string) ([]Member, []string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var members []Member
	var warnings []string
	for _, entry := range entries {
		var path string
		if entry.IsDir() {
			path = filepath.Join(root, entry.Name(), MemberEntryFile)
		} else if strings.HasSuffix(entry.Name(), ".md") {
			path = filepath.Join(root, entry.Name())
		} else {
			continue
		}
		member, err := parseMemberFile(path)
		if err != nil {
			warnings = append(warnings, "member config skipped: "+err.Error())
			continue
		}
		members = append(members, member)
	}
	return members, warnings, nil
}

func parseMemberFile(path string) (Member, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Member{}, err
	}
	values := parseFrontmatter(string(raw))
	return Member{
		Name:             values["name"],
		Role:             values["role"],
		InstanceID:       values["instance_id"],
		Workdir:          values["workdir"],
		Backend:          Backend(values["backend"]),
		RequiresApproval: parseFrontmatterBool(values["requires_approval"]),
		Status:           MemberStatusIdle,
		ResumeRef:        values["resume_ref"],
	}, nil
}

func parseFrontmatter(body string) map[string]string {
	values := map[string]string{}
	lines := strings.Split(body, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return values
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}

func parseFrontmatterBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(value)))
	return err == nil && parsed
}
