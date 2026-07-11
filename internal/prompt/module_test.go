package prompt

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSortsByPriorityThenIDAndIsStable(t *testing.T) {
	modules := []Module{
		{ID: "style", Priority: 20, Content: "style"},
		{ID: "identity", Priority: 10, Content: "identity"},
		{ID: "behavior", Priority: 10, Content: "behavior"},
	}
	want := "behavior\n\nidentity\n\nstyle"
	for i := 0; i < 10; i++ {
		if got := Build(modules); got != want {
			t.Fatalf("Build = %q, want %q", got, want)
		}
	}
}

func TestDefaultModulesContainExpectedResponsibilities(t *testing.T) {
	got := StableGlobalInstruction()
	for _, want := range []string{"identity", "behavior", "tool", "code", "safety", "mode", "style"} {
		if !strings.Contains(strings.ToLower(moduleIDs()), want) {
			t.Fatalf("default modules missing %s", want)
		}
	}
	for _, want := range []string{"优先使用专用工具", "不要优先使用 run_command", "编辑前先读取相关文件"} {
		if !strings.Contains(got, want) {
			t.Fatalf("global instruction missing %q: %s", want, got)
		}
	}
}

func TestEnvironmentMessageIsDynamicButGlobalInstructionIsStable(t *testing.T) {
	stableA := StableGlobalInstruction()
	stableB := StableGlobalInstruction()
	if stableA != stableB {
		t.Fatalf("stable global instruction changed")
	}
	if strings.Contains(stableA, "/tmp/project") || strings.Contains(stableA, "2026-07-06") || strings.Contains(stableA, "main") {
		t.Fatalf("stable global instruction contains dynamic data: %s", stableA)
	}

	envA := EnvironmentMessage(Environment{CWD: "/tmp/project", OS: "darwin", Time: time.Date(2026, 7, 6, 1, 0, 0, 0, time.UTC), Git: "main"})
	envB := EnvironmentMessage(Environment{CWD: "/tmp/project", OS: "darwin", Time: time.Date(2026, 7, 6, 2, 0, 0, 0, time.UTC), Git: "main"})
	for _, want := range []string{"cwd", "os", "time", "git"} {
		if !strings.Contains(envA.Content, want) {
			t.Fatalf("environment message missing %s: %s", want, envA.Content)
		}
	}
	if envA.Content == envB.Content {
		t.Fatalf("environment message did not change with time")
	}
}

func TestInternalInstructionAndPlanOnlyReminder(t *testing.T) {
	message := InternalInstruction("外部工具上线")
	if !strings.Contains(message.Content, InstructionOpenTag) || !strings.Contains(message.Content, InstructionCloseTag) {
		t.Fatalf("message missing instruction tags: %s", message.Content)
	}
	if strings.Contains(message.Content, "<user>") {
		t.Fatalf("internal instruction looked like user input")
	}

	full := PlanOnlyReminder(1)
	fullFive := PlanOnlyReminder(5)
	fullTen := PlanOnlyReminder(10)
	short := PlanOnlyReminder(2)
	if !strings.Contains(full.Content, "只允许读类工具") || !strings.Contains(full.Content, "最终输出计划供用户审批") {
		t.Fatalf("full reminder missing constraints: %s", full.Content)
	}
	if !strings.Contains(fullFive.Content, "只允许读类工具") {
		t.Fatalf("fifth reminder should be full: %s", fullFive.Content)
	}
	if !strings.Contains(fullTen.Content, "只允许读类工具") {
		t.Fatalf("tenth reminder should be full: %s", fullTen.Content)
	}
	for _, iteration := range []int{2, 3, 4} {
		short = PlanOnlyReminder(iteration)
		if len(short.Content)*2 >= len(full.Content) {
			t.Fatalf("short reminder for iteration %d too long: short=%d full=%d", iteration, len(short.Content), len(full.Content))
		}
	}
	if ShouldInjectPlanOnly(false) {
		t.Fatalf("plan-only reminder injected while disabled")
	}
}

func moduleIDs() string {
	var ids []string
	for _, module := range DefaultModules() {
		ids = append(ids, module.ID)
	}
	return strings.Join(ids, " ")
}
