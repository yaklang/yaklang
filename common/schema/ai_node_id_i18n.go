package schema

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type I18n struct {
	Zh string `json:"zh"`
	En string `json:"en"`
}

var nodeIdMapper = map[string]*I18n{
	"next_movements": {
		Zh: "下一步行动",
		En: "Next Movements",
	},
	"self-reflection-suggestions": {
		Zh: "自省建议",
		En: "Self Reflection Suggestions",
	},
	"semantic_questions": {
		Zh: "语义搜索条件",
		En: "Semantic Search Questions",
	},
	"search_patterns": {
		Zh: "代码搜索条件",
		En: "Search Patterns",
	},
	"semantic_search_code": {
		Zh: "代码语义搜索",
		En: "Semantic Code Search",
	},
	"init-search-code-sample": {
		Zh: "初始化代码样本搜索",
		En: "Initialize Code Sample Search",
	},
	"code_sample_title": {
		Zh: "代码样本",
		En: "Code Sample",
	},
	"mcp-loader": {
		Zh: "MCP加载器",
		En: "MCP Loader",
	},
	"java_decompiler": {
		Zh: "Java反编译器",
		En: "Java Decompiler",
	},
	"fast-memory-fetch": {
		Zh: "快速记忆检索",
		En: "Fast Memory Fetch",
	},
	"grep_yaklang_samples": {
		Zh: "搜索Yaklang样本",
		En: "Search Yaklang Samples",
	},
	"search_yaklang_samples": {
		Zh: "RAG 语义搜索Yaklang样本",
		En: "RAG Semantic Search Yaklang Samples",
	},
	"grep_samples_result": {
		Zh: "样本搜索结果",
		En: "Sample Search Result",
	},
	"query_yaklang_document": {
		Zh: "查询 Yaklang 文档",
		En: "Query Yaklang Document",
	},
	"batch-compress": {
		Zh: "记忆压缩",
		En: "Memory Compression",
	},
	"write_yaklang_code": {
		Zh: "编写 Yaklang 代码",
		En: "Writing Yaklang",
	},
	"default": {
		Zh: "通用",
		En: "General",
	},
	// 从注释中的 AIStreamNodeIdToLabel 获取的 nodeId
	"re-act-loop": {
		Zh: "推理与行动",
		En: "Reason and Act",
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
	"thought": {
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
		Zh: "专注",
		En: "Focus On",
	},
	"lose-focus": {
		Zh: "通用模式",
		En: "Lose Focus",
	},
	"react_task_status_changed": {
		Zh: "任务状态变更",
		En: "ReAct Task Status Changed",
	},
	"react_task_enqueue": {
		Zh: "任务入队",
		En: "ReAct Task Enqueue",
	},
	"react_task_dequeue": {
		Zh: "任务出队",
		En: "ReAct Task Dequeue",
	},
	"react_task_created": {
		Zh: "任务创建",
		En: "ReAct Task Created",
	},
	"react_task_cleared": {
		Zh: "任务队列清空",
		En: "ReAct Task queue Cleared",
	},
	// 日志级别相关的 nodeId
	"info": {
		Zh: "信息",
		En: "Info",
	},
	"warning": {
		Zh: "警告",
		En: "Warning",
	},
	"error": {
		Zh: "错误",
		En: "Error",
	},
	"debug": {
		Zh: "调试",
		En: "Debug",
	},
	"trace": {
		Zh: "跟踪",
		En: "Trace",
	},
	"fatal": {
		Zh: "致命错误",
		En: "Fatal",
	},

	// 动态生成的工具相关 nodeId 模式
	"tool-stdout": {
		Zh: "工具标准输出",
		En: "Tool Standard Output",
	},
	"tool-stderr": {
		Zh: "工具标准错误",
		En: "Tool Standard Error",
	},

	// 工具调用相关的具体 nodeId (这些是动态生成的，但可以提供通用翻译)
	"tool-print-stdout": {
		Zh: "打印工具输出",
		En: "Print Tool Output",
	},
	"tool-print-stderr": {
		Zh: "打印工具错误",
		En: "Print Tool Error",
	},

	// 其他在代码中发现的 nodeId
	"event_trigger_node": {
		Zh: "事件触发节点",
		En: "Event Trigger Node",
	},
	"mirror_trigger_node": {
		Zh: "镜像触发节点",
		En: "Mirror Trigger Node",
	},
	"scan-progress": {
		Zh: "扫描进度",
		En: "Scan Progress",
	},
	"plan_error": {
		Zh: "计划错误",
		En: "Plan Error",
	},
	"plan_warning": {
		Zh: "计划警告",
		En: "Plan Warning",
	},
	"plan_execution": {
		Zh: "计划执行",
		En: "Plan Execution",
	},
	"tool_execution": {
		Zh: "工具执行",
		En: "Tool Execution",
	},
	"directly_answer_required": {
		Zh: "需要直接回答",
		En: "Direct Answer Required",
	},
	"re-select-tool": {
		Zh: "重新选择工具",
		En: "Re-select Tool",
	},
	"re-select-tool-failed": {
		Zh: "重新选择工具失败",
		En: "Re-select Tool Failed",
	},
	"error-calling-tool": {
		Zh: "调用工具错误",
		En: "Error Calling Tool",
	},
	"document_query_results": {
		Zh: "文档查询结果",
		En: "Document Query Results",
	},
	"gen_code": {
		Zh: "生成代码",
		En: "Generate Code",
	},
	"tool-result-history": {
		Zh: "工具结果历史",
		En: "Tool Result History",
	},
	"tool-params": {
		Zh: "工具参数",
		En: "Tool Parameters",
	},
	"tool-result": {
		Zh: "工具结果",
		En: "Tool Result",
	},
	"verification": {
		Zh: "验证",
		En: "Verification",
	},
	"answer": {
		Zh: "回答",
		En: "Answer",
	},
	"base": {
		Zh: "基础",
		En: "Base",
	},
	"wrong-tool": {
		Zh: "错误工具",
		En: "Wrong Tool",
	},

	// AddToTimeline 中使用的 nodeId (去除重复项)
	"directly-answer": {
		Zh: "直接回答",
		En: "Direct Answer",
	},
	"finish": {
		Zh: "完成",
		En: "Finish",
	},
	"USER-Original-Query": {
		Zh: "用户原始查询",
		En: "User Original Query",
	},
	"code_modified": {
		Zh: "代码修改",
		En: "Code Modified",
	},
	"note": {
		Zh: "备注",
		En: "Note",
	},

	// 工具调用相关的 timeline nodeId
	"call[*] error": {
		Zh: "调用错误",
		En: "Call Error",
	},

	// 其他发现的 nodeId
	"re-act-runtime": {
		Zh: "ReAct 运行时",
		En: "ReAct Runtime",
	},
	"react-task": {
		Zh: "ReAct 任务",
		En: "ReAct Task",
	},

	// 自省相关的 nodeId
	"self-reflection": {
		Zh: "自我反思",
		En: "Self Reflection",
	},
	"self-reflection-learning_insights": {
		Zh: "学习洞察",
		En: "Learning Insights",
	},
	"self-reflection-future_suggestions": {
		Zh: "未来建议",
		En: "Future Suggestions",
	},
	"self-reflection-impact_assessment": {
		Zh: "影响评估",
		En: "Impact Assessment",
	},
	"self-reflection-effectiveness_rating": {
		Zh: "效果评级",
		En: "Effectiveness Rating",
	},
	"yaklang-code": {
		Zh: "Yaklang 代码",
		En: "Yaklang Code",
	},
}

func NodeIdToI18n(nodeId string, isStream bool) *I18n {
	if val, ok := nodeIdMapper[nodeId]; ok {
		return val
	}
	if isStream {
		if strings.HasPrefix(nodeId, "tool-") {
			if strings.HasSuffix(nodeId, "-stdout") {
				return &I18n{
					Zh: "工具标准输出流",
					En: "Tool Standard Output",
				}
			} else if strings.HasSuffix(nodeId, "-stderr") {
				return &I18n{
					Zh: "工具标准错误流",
					En: "Tool Standard Error",
				}
			}
		}
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
