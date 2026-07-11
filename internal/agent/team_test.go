package agent

import (
	"testing"

	"mewcode/internal/team"
	"mewcode/internal/tool"
)

func TestTeamToolsHiddenByDefaultAndVisibleForLead(t *testing.T) {
	defs := []tool.Definition{
		{Name: "read_file"},
		{Name: team.ToolTaskCreate},
		{Name: team.ToolMessageSend},
	}
	withoutManager := (&Agent{Tools: defs}).toolsForTurn()
	if len(withoutManager) != 1 || withoutManager[0].Name != "read_file" {
		t.Fatalf("without manager defs = %#v", withoutManager)
	}
	manager := team.NewManager(t.TempDir(), team.Options{})
	main := (&Agent{Tools: defs, TeamManager: manager}).toolsForTurn()
	if len(main) != 1 || main[0].Name != "read_file" {
		t.Fatalf("main defs = %#v", main)
	}
	lead := (&Agent{Tools: defs, TeamManager: manager, TeamActor: team.Actor{Team: "alpha", Name: "lead", Kind: team.ActorLead}}).toolsForTurn()
	if len(lead) != 3 {
		t.Fatalf("lead defs = %#v", lead)
	}
}
