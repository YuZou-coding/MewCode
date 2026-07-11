package prompt

import (
	"strings"

	"mewcode/internal/chat"
)

const (
	InstructionOpenTag  = "<mewcode-instruction>"
	InstructionCloseTag = "</mewcode-instruction>"
)

func InternalInstruction(content string) chat.Message {
	return chat.Message{
		Role:    chat.RoleSystem,
		Content: InstructionOpenTag + "\n" + strings.TrimSpace(content) + "\n" + InstructionCloseTag,
	}
}
