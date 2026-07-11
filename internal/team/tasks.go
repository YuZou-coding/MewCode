package team

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (m *Manager) CreateTask(teamName, title, description, assignee string, dependsOn []string) (Task, error) {
	if strings.TrimSpace(title) == "" {
		return Task{}, fmt.Errorf("task title is required")
	}
	tasks, path, err := m.loadTasks(teamName)
	if err != nil {
		return Task{}, err
	}
	now := time.Now()
	task := Task{
		ID:          fmt.Sprintf("task_%d", len(tasks)+1),
		Title:       title,
		Description: description,
		Assignee:    assignee,
		Status:      TaskStatusOpen,
		DependsOn:   compactStrings(dependsOn),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	tasks = append(tasks, task)
	return task, writeJSON(path, tasks)
}

func (m *Manager) GetTask(teamName, id string) (Task, error) {
	tasks, _, err := m.loadTasks(teamName)
	if err != nil {
		return Task{}, err
	}
	for _, task := range tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return Task{}, fmt.Errorf("task not found: %s", id)
}

func (m *Manager) ListTasks(teamName string) ([]Task, error) {
	tasks, _, err := m.loadTasks(teamName)
	if err != nil {
		return nil, err
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt.Before(tasks[j].CreatedAt) })
	return tasks, nil
}

func (m *Manager) UpdateTask(teamName, id string, status TaskStatus, result, assignee string) (Task, error) {
	tasks, path, err := m.loadTasks(teamName)
	if err != nil {
		return Task{}, err
	}
	for i := range tasks {
		if tasks[i].ID != id {
			continue
		}
		if status != "" {
			tasks[i].Status = status
		}
		if result != "" {
			tasks[i].Result = result
		}
		if assignee != "" {
			tasks[i].Assignee = assignee
		}
		tasks[i].UpdatedAt = time.Now()
		return tasks[i], writeJSON(path, tasks)
	}
	return Task{}, fmt.Errorf("task not found: %s", id)
}

func (m *Manager) IncompleteTaskCount(teamName string) int {
	tasks, err := m.ListTasks(teamName)
	if err != nil {
		return 0
	}
	count := 0
	for _, task := range tasks {
		if task.Status != TaskStatusDone {
			count++
		}
	}
	return count
}

func (m *Manager) loadTasks(teamName string) ([]Task, string, error) {
	team, err := m.Load(teamName)
	if err != nil {
		return nil, "", err
	}
	path := filepath.Join(team.Root, "tasks.json")
	var tasks []Task
	if err := readJSON(path, &tasks); err != nil {
		return nil, "", err
	}
	return tasks, path, nil
}

func compactStrings(items []string) []string {
	var out []string
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
