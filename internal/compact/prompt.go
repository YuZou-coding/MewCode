package compact

import (
	"strings"

	"mewcode/internal/chat"
)

var SummarySections = []string{
	"主要请求",
	"关键概念",
	"文件代码",
	"错误修复",
	"解决过程",
	"用户原话",
	"待办",
	"当前工作",
	"下一步",
}

const noToolsInstruction = "禁止调用工具"

func SummaryPrompt(messages []chat.Message) string {
	var b strings.Builder
	b.WriteString(noToolsInstruction + "。你正在执行上下文压缩摘要任务，绝对不要请求或调用任何工具。\n")
	b.WriteString("先输出【分析草稿】帮助你梳理旧上下文，然后输出【正式摘要】。分析草稿只供你组织信息，调用方会丢弃草稿。\n")
	b.WriteString("正式摘要必须包含以下固定区块：\n")
	for _, section := range SummarySections {
		b.WriteString("- " + section + "\n")
	}
	b.WriteString("\n待压缩历史：\n")
	for _, message := range messages {
		b.WriteString("[" + string(message.Role) + "] ")
		if message.ToolResult != nil {
			b.WriteString("tool=" + message.ToolResult.Name + " call_id=" + message.ToolResult.CallID + " content=" + string(message.ToolResult.Content))
		} else {
			b.WriteString(message.Content)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + noToolsInstruction + "。只输出分析草稿和正式摘要，不要调用任何工具。")
	return b.String()
}

func ExtractOfficialSummary(text string) string {
	markers := []string{"【正式摘要】", "正式摘要:", "正式摘要："}
	for _, marker := range markers {
		if index := strings.LastIndex(text, marker); index >= 0 {
			return strings.TrimSpace(text[index+len(marker):])
		}
	}
	return strings.TrimSpace(text)
}

func HasAllSummarySections(text string) bool {
	for _, section := range SummarySections {
		if !strings.Contains(text, section) {
			return false
		}
	}
	return true
}
