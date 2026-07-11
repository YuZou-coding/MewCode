package command

import (
	"context"
	"strings"
)

type SkillCommand struct {
	Name        string
	Description string
	Mode        string
}

func RegisterSkillCommands(r *Registry, skills []SkillCommand) error {
	for _, skill := range skills {
		name := normalizeName(skill.Name)
		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = "运行 Skill " + name
		}
		cmd := Command{
			Name:        name,
			Description: description,
			Usage:       "/" + name + " [args]",
			Type:        TypeAIPrompt,
			ArgHint:     "args",
			Handler: func(skillName string, mode string) Handler {
				return func(ctx context.Context, controller Controller, inv Invocation) Result {
					prompt, err := controller.RunSkill(ctx, skillName, inv.Args)
					if err != nil {
						return Result{Err: err}
					}
					if mode == "isolated" {
						return Message(prompt)
					}
					return Result{SendToAgent: prompt}
				}
			}(name, skill.Mode),
		}
		if err := r.Register(cmd); err != nil {
			return err
		}
	}
	return nil
}
