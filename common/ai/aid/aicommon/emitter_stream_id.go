package aicommon

type I18n struct {
	Zh string
	En string
}

/*
export const AIStreamNodeIdToLabel: Record<string, {label: string}> = {
    "re-act-loop": {label: "推理与行动"},
    "call-forge": {label: "智能应用"},
    "call-tools": {label: "工具调用"},
    review: {label: "审查系统"},
    liteforge: {label: "轻量智能应用"},
    directly_answer: {label: "直接回答"},
    "memory-reducer": {label: "记忆裁剪"},
    "memory-timeline": {label: "记忆浓缩"},
    execute: {label: "执行"},
    summary: {label: "总结"},
    "create-subtasks": {label: "创建子任务"},
    "freedom-plan-review": {label: "计划审查"},
    "dynamic-plan": {label: "动态规划"},
    "re-act-verify": {label: "核实结果"},
    result: {label: "结果输出"},
    plan: {label: "任务规划"},
    decision: {label: "决策"},
    output: {label: "通用输出"},
    forge: {label: "智能应用"},
    "re-act-loop-thought": {label: "思考"},
    "re-act-loop-answer-payload": {label: "AI 响应"},
    "enhance-query": {label: "知识增强"}
}
*/

var nodeIdMapper = map[string]I18n{
	"re-act-loop-thought": {
		Zh: "思考",
		En: "Thought",
	},
}

func NodeIdToI18n(nodeId string) I18n {
	if val, ok := nodeIdMapper[nodeId]; ok {
		return val
	}
	return I18n{
		Zh: nodeId,
		En: nodeId,
	}
}
