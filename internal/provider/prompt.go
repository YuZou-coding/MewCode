package provider

import (
	"mewcode/internal/chat"
	"mewcode/internal/prompt"
	"mewcode/internal/tool"
)

func providerSystemAndMessages(messages []chat.Message, tools []tool.Definition) (string, []chat.Message) {
	normal := make([]chat.Message, len(messages))
	copy(normal, messages)
	return prompt.StableGlobalInstruction(), normal
}
