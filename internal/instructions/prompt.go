package instructions

import (
	"fmt"
	"strings"

	"mewcode/internal/chat"
	"mewcode/internal/prompt"
)

func Messages(blocks []Block) []chat.Message {
	messages := make([]chat.Message, 0, len(blocks))
	for _, block := range blocks {
		content := strings.TrimSpace(block.Content)
		if content == "" {
			continue
		}
		messages = append(messages, prompt.InternalInstruction(fmt.Sprintf("MewCode %s 指令：\n%s", block.Source, content)))
	}
	return messages
}
