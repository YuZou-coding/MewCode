package prompt

import "mewcode/internal/chat"

func PlanOnlyReminder(iteration int) chat.Message {
	if fullPlanOnlyReminder(iteration) {
		return InternalInstruction("当前处于 plan-only 模式：只允许读类工具；不要执行写入、编辑或命令；最终输出计划供用户审批。")
	}
	return InternalInstruction("plan-only：只读，给计划。")
}

func ShouldInjectPlanOnly(planOnly bool) bool {
	return planOnly
}

func fullPlanOnlyReminder(iteration int) bool {
	return iteration == 1 || iteration%5 == 0
}
