package compact

import (
	"encoding/json"

	"mewcode/internal/chat"
)

func MessageSize(message chat.Message) int {
	size := len([]rune(message.Content))
	if message.ToolResult != nil {
		size += ToolResultSize(*message.ToolResult)
	}
	for _, call := range message.ToolCalls {
		size += len([]rune(call.Name)) + len([]rune(string(call.Arguments)))
	}
	if message.ToolCall != nil && len(message.ToolCalls) == 0 {
		size += len([]rune(message.ToolCall.Name)) + len([]rune(string(message.ToolCall.Arguments)))
	}
	return size
}

func ToolResultSize(result chat.ToolResult) int {
	return len([]rune(string(result.Content)))
}

func ToolResultsTotal(messages []chat.Message) int {
	total := 0
	for _, message := range messages {
		if message.ToolResult != nil {
			total += ToolResultSize(*message.ToolResult)
		}
	}
	return total
}

func HistorySize(messages []chat.Message) int {
	total := 0
	for _, message := range messages {
		total += MessageSize(message)
	}
	return total
}

func CloneMessages(messages []chat.Message) []chat.Message {
	copied := make([]chat.Message, len(messages))
	copy(copied, messages)
	for i := range copied {
		if messages[i].ToolResult != nil {
			result := *messages[i].ToolResult
			result.Content = append(json.RawMessage(nil), messages[i].ToolResult.Content...)
			copied[i].ToolResult = &result
		}
		if messages[i].ToolCall != nil {
			call := *messages[i].ToolCall
			call.Arguments = append(json.RawMessage(nil), messages[i].ToolCall.Arguments...)
			copied[i].ToolCall = &call
		}
		if len(messages[i].ToolCalls) > 0 {
			copied[i].ToolCalls = make([]chat.ToolCall, len(messages[i].ToolCalls))
			copy(copied[i].ToolCalls, messages[i].ToolCalls)
			for j := range copied[i].ToolCalls {
				copied[i].ToolCalls[j].Arguments = append(json.RawMessage(nil), messages[i].ToolCalls[j].Arguments...)
			}
			if copied[i].ToolCall != nil {
				copied[i].ToolCall = &copied[i].ToolCalls[0]
			}
		}
	}
	return copied
}
