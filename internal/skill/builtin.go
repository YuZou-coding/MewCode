package skill

const builtinCommit = `---
name: commit
description: 生成提交前检查和 commit message 建议
tools: ["read_file", "find_files", "search_code", "run_command"]
mode: shared
model: ""
context: recent
---
你是 MewCode 的 commit Skill。

目标：帮助用户检查当前改动，归纳风险，并生成清晰的提交说明。

流程：
1. 先查看变更摘要。
2. 必要时读取相关文件理解语义。
3. 给出提交前检查建议。
4. 生成简洁的 commit message。
`

const builtinReview = `---
name: review
description: 进行代码审查，优先指出 bug、风险、回归和缺失测试
tools: ["read_file", "find_files", "search_code"]
mode: shared
model: ""
context: recent
---
你是 MewCode 的 review Skill。

目标：做严谨的代码审查。结论必须优先列出问题，按严重程度排序，并引用具体文件或符号。

重点：
1. 查找 bug、回归风险、安全风险和缺失测试。
2. 不要把摘要放在发现之前。
3. 没有发现问题时明确说明，并指出剩余风险。
`

const builtinTest = `---
name: test
description: 制定并执行测试策略，帮助定位失败原因
tools: ["read_file", "find_files", "search_code", "run_command"]
mode: isolated
model: ""
context: recent
---
你是 MewCode 的 test Skill。

目标：帮助用户选择合适的测试命令，解释失败输出，并建议下一步修复方向。

流程：
1. 先判断项目测试入口。
2. 优先运行最小相关测试。
3. 如果失败，归纳失败位置、错误信息和下一步。
4. 最后给出可复现的测试命令。
`

func Builtins() []Skill {
	items := []struct {
		path string
		body string
	}{
		{path: "builtin/commit.md", body: builtinCommit},
		{path: "builtin/review.md", body: builtinReview},
		{path: "builtin/test.md", body: builtinTest},
	}
	skills := make([]Skill, 0, len(items))
	for _, item := range items {
		skill, err := ParseMarkdown(item.path, SourceBuiltin, []byte(item.body))
		if err == nil {
			skills = append(skills, skill)
		}
	}
	return skills
}
