package schema

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

var nodeIdMapper = map[string]*I18n{
	// 从注释中的 AIStreamNodeIdToLabel 获取的 nodeId
	"re-act-loop": {
		Zh: "推理与行动",
		En: "ReAct Loop",
	},
	"call-forge": {
		Zh: "智能应用",
		En: "AI Forge",
	},
	"call-tools": {
		Zh: "工具调用",
		En: "Tool Call",
	},
	"review": {
		Zh: "审查系统",
		En: "Review System",
	},
	"liteforge": {
		Zh: "轻量智能应用",
		En: "Lite Forge",
	},
	"directly_answer": {
		Zh: "直接回答",
		En: "Direct Answer",
	},
	"memory-reducer": {
		Zh: "记忆裁剪",
		En: "Memory Reducer",
	},
	"memory-timeline": {
		Zh: "记忆浓缩",
		En: "Memory Timeline",
	},
	"execute": {
		Zh: "执行",
		En: "Execute",
	},
	"summary": {
		Zh: "总结",
		En: "Summary",
	},
	"create-subtasks": {
		Zh: "创建子任务",
		En: "Create Subtasks",
	},
	"freedom-plan-review": {
		Zh: "计划审查",
		En: "Plan Review",
	},
	"dynamic-plan": {
		Zh: "动态规划",
		En: "Dynamic Plan",
	},
	"re-act-verify": {
		Zh: "核实结果",
		En: "Verify Results",
	},
	"result": {
		Zh: "结果输出",
		En: "Result Output",
	},
	"plan": {
		Zh: "任务规划",
		En: "Task Planning",
	},
	"decision": {
		Zh: "决策",
		En: "Decision",
	},
	"output": {
		Zh: "通用输出",
		En: "General Output",
	},
	"forge": {
		Zh: "智能应用",
		En: "AI Forge",
	},
	"re-act-loop-thought": {
		Zh: "思考",
		En: "Thought",
	},
	"re-act-loop-answer-payload": {
		Zh: "AI 响应",
		En: "AI Response",
	},
	"enhance-query": {
		Zh: "知识增强",
		En: "Knowledge Enhancement",
	},

	// 从代码中找到的其他 nodeId
	"action": {
		Zh: "行动",
		En: "Action",
	},
	"iteration": {
		Zh: "迭代",
		En: "Iteration",
	},
	"knowledge": {
		Zh: "知识",
		En: "Knowledge",
	},
	"yakit": {
		Zh: "Yakit 执行结果",
		En: "Yakit Execution Result",
	},
	"status": {
		Zh: "状态",
		En: "Status",
	},
	"permission": {
		Zh: "权限",
		En: "Permission",
	},
	"review-require": {
		Zh: "审查请求",
		En: "Review Required",
	},
	"review-release": {
		Zh: "审查释放",
		En: "Review Release",
	},
	"filesystem": {
		Zh: "文件系统",
		En: "File System",
	},
	"stream-finished": {
		Zh: "流结束",
		En: "Stream Finished",
	},
	"prompt": {
		Zh: "提示",
		En: "Prompt",
	},
	"system": {
		Zh: "系统",
		En: "System",
	},
	"ai-reviewer": {
		Zh: "AI 审查员",
		En: "AI Reviewer",
	},
	"queue_info": {
		Zh: "队列信息",
		En: "Queue Info",
	},
	"timeline": {
		Zh: "时间线",
		En: "Timeline",
	},
	"risk-control": {
		Zh: "风险控制",
		En: "Risk Control",
	},
	"task-analyst": {
		Zh: "任务分析",
		En: "Task Analyst",
	},
	"write_code": {
		Zh: "编写代码",
		En: "Write Code",
	},
	"modify_code": {
		Zh: "修改代码",
		En: "Modify Code",
	},
	"query_document": {
		Zh: "查询文档",
		En: "Query Document",
	},
	"focus-on": {
		Zh: "聚焦",
		En: "Focus On",
	},
	"lose-focus": {
		Zh: "失去焦点",
		En: "Lose Focus",
	},
	"react_task_status_changed": {
		Zh: "ReAct 任务状态变更",
		En: "ReAct Task Status Changed",
	},
	"react_task_enqueue": {
		Zh: "ReAct 任务入队",
		En: "ReAct Task Enqueue",
	},
	"react_task_dequeue": {
		Zh: "ReAct 任务出队",
		En: "ReAct Task Dequeue",
	},
	"react_task_created": {
		Zh: "ReAct 任务创建",
		En: "ReAct Task Created",
	},
}

func NodeIdToI18n(nodeId string, isStream bool) *I18n {
	if val, ok := nodeIdMapper[nodeId]; ok {
		return val
	}
	if isStream {
		log.Warn("================================================")
		log.Warnf("[i18n] nodeId cannot be found in nodeIdMapper: %s", nodeId)
		log.Warn("================================================")
	}
	return &I18n{
		Zh: nodeId,
		En: nodeId,
	}
}

func (i18n *I18n) I18nToYPB_I18n() *ypb.I18N {
	return &ypb.I18N{
		Zh: i18n.Zh,
		En: i18n.En,
	}
}
