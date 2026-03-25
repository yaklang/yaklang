package schema

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type I18n struct {
	Zh string `json:"zh"`
	En string `json:"en"`
}

type i18nCacheEntry struct {
	once   sync.Once
	result *I18n
}

var i18nDynamicCache sync.Map

const i18nCacheKeyPrefix = "ai_i18n_node_id_"

func getNodeIdI18nFromDB(nodeId string) *I18n {
	db := GetGormProfileDatabase()
	if db == nil {
		return nil
	}
	keyStr := strconv.Quote(i18nCacheKeyPrefix + nodeId)
	var storage GeneralStorage
	if err := db.Where("key = ?", keyStr).First(&storage).Error; err != nil {
		return nil
	}
	val := storage.Value
	if val == "" {
		return nil
	}
	unquoted, err := strconv.Unquote(val)
	if err != nil {
		unquoted = val
	}
	var result I18n
	if err := json.Unmarshal([]byte(unquoted), &result); err != nil {
		return nil
	}
	if result.Zh == "" && result.En == "" {
		return nil
	}
	return &result
}

func saveNodeIdI18nToDB(nodeId string, i18n *I18n) {
	db := GetGormProfileDatabase()
	if db == nil {
		return
	}
	raw, err := json.Marshal(i18n)
	if err != nil {
		return
	}
	keyStr := strconv.Quote(i18nCacheKeyPrefix + nodeId)
	valueStr := strconv.Quote(string(raw))
	db.Model(&GeneralStorage{}).Where("key = ?", keyStr).Assign(map[string]interface{}{
		"key": keyStr, "value": valueStr,
	}).FirstOrCreate(&GeneralStorage{})
}

var nodeIdMapper = map[string]*I18n{
	"loading_skills_names": {
		Zh: "加载技能列表",
		En: "Loading Skills List",
	},
	"load_skill_resources_path": {
		Zh: "额外加载技能资源",
		En: "Load Extra SKILL Resource",
	},
	"internet_research": {
		Zh: "互联网调研",
		En: "Internet Research",
	},
	"internet-research": {
		Zh: "互联网调研",
		En: "Internet Research",
	},
	"load_capability": {
		Zh: "加载能力",
		En: "Loading Capabilities",
	},
	"loading_skills_name": {
		Zh: "加载技能",
		En: "Loading Skills",
	},
	"intent": {
		Zh: "意图识别",
		En: "Intent Recognition",
	},
	"intent_analysis": {
		Zh: "意图分析",
		En: "Intent Analysis",
	},
	"intent_finalized": {
		Zh: "意图识别完成",
		En: "Intent Finalized",
	},
	"intent_recognition_finalized": {
		Zh: "意图识别完成",
		En: "Intent Recognition Finalized",
	},
	"intent_recommended_tools": {
		Zh: "推荐工具",
		En: "Recommended Tools",
	},
	"intent_recommended_forges": {
		Zh: "推荐蓝图",
		En: "Recommended Forges",
	},
	"intent_context_enrichment": {
		Zh: "能力上下文",
		En: "Capability Context",
	},
	"search_capabilities": {
		Zh: "能力搜索",
		En: "Capability Search",
	},
	"search_capabilities_start": {
		Zh: "开始搜索能力",
		En: "Capability Search Started",
	},
	"search_capabilities_analysis": {
		Zh: "能力意图分析",
		En: "Capability Intent Analysis",
	},
	"search_capabilities_completed": {
		Zh: "能力搜索完成",
		En: "Capability Search Completed",
	},
	"load_capability_intent_analysis": {
		Zh: "能力候选分析",
		En: "Capability Candidate Analysis",
	},
	"[LOAD_CAPABILITY_INTENT_DONE]": {
		Zh: "能力候选已识别",
		En: "Capability Candidates Identified",
	},
	"web_search_recommended": {
		Zh: "建议使用网页搜索",
		En: "Web Search Recommended",
	},
	"semantic_search_yaklang_samples": {
		Zh: "Yaklang样本语义搜索",
		En: "Yaklang Samples Semantic Search",
	},
	"search-relative-knowledge-base": {
		Zh: "选择相关知识库",
		En: "Search Relative Knowledge Base",
	},
	"rewrite_user_query_for_knowledge_enhance": {
		Zh: "知识增强：重写用户查询",
		En: "Knowledge Enhance: Rewrite User Query",
	},
	"interval-review": {
		Zh: "进度检查",
		En: "Progress Check",
	},
	"tool_compose_progress": {
		Zh: "工具编排",
		En: "Tool Compose",
	},
	"knowledge-compress": {
		Zh: "知识压缩",
		En: "Knowledge Compress",
	},
	"reference_material": {
		Zh: "参考资料",
		En: "Reference Material",
	},
	"human_readable_result": {
		Zh: "结果摘要",
		En: "Human Readable Result",
	},
	"next_movements": {
		Zh: "下一步行动",
		En: "Next Movements",
	},
	"next_movements_snapshot": {
		Zh: "待办事务",
		En: "TODO",
	},
	"delivery_files_snapshot": {
		Zh: "交付文件",
		En: "Delivery Files",
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
	"generating-tool-call-params": {
		Zh: "生成工具参数",
		En: "Generating Params",
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
	"summary-status": {
		Zh: "状态总结",
		En: "Status Summary",
	},
	"summary-short": {
		Zh: "简短总结",
		En: "Short Summary",
	},
	"summary-long": {
		Zh: "任务总结",
		En: "Task Summary",
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
	"main-plan": {
		Zh: "主任务",
		En: "Main Plan",
	},
	"plan-review": {
		Zh: "任务规划反思",
		En: "Plan Review",
	},
	"task-review": {
		Zh: "任务反思",
		En: "Task Review",
	},
	"main-plan-goal": {
		Zh: "主任务目标",
		En: "Main Plan Target",
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
	"plan-executing-loading-status-key": {
		Zh: "计划执行状态",
		En: "Plan Execution Status",
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
	"react_task_mode_changed": {
		Zh: "任务执行模式变更",
		En: "Task Execution Mode Changed",
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
	"report_generating": {
		Zh: "报告生成",
		En: "Report Generating",
	},
	"report-content": {
		Zh: "报告内容",
		En: "Report Content",
	},
	"python-script": {
		Zh: "Python 脚本",
		En: "Python Script",
	},
	"write_python_script": {
		Zh: "编写 Python 脚本",
		En: "Write Python Script",
	},
	"plan_exec_fail": {
		Zh: "任务规划执行失败",
		En: "plan and execute failed",
	},
	"re_act_fail": {
		Zh: "ReAct 任务执行失败",
		En: "ReAct Task Execution Failed",
	},
	"re_act_success": {
		Zh: "ReAct 任务执行成功",
		En: "ReAct Task Execution Success",
	},
	"knowledge_enhance": {
		Zh: "知识增强",
		En: "Knowledge Enhancement",
	},
	"knowledge-single-relevance": {
		Zh: "知识相关性分析",
		En: "Knowledge Relevance Analysis",
	},
	"knowledge-chunk-relevance": {
		Zh: "知识分片相关性分析",
		En: "Knowledge Chunk Relevance Analysis",
	},
	"http_flow_analyze": {
		Zh: "HTTP 流量分析",
		En: "HTTP Flow Analyze",
	},
}

var eventTypeMapper = map[EventType]*I18n{
	EVENT_TOOL_CALL_RESULT: {
		Zh: "工具结果",
		En: "Tool Result",
	},
}

// EnsureStreamNodeIdI18n ensures a stream nodeId has i18n translation available
// before events are emitted. The provider is injected by the Emitter and uses
// the Config's own AI callback, so mock callbacks in tests automatically cover it.
func EnsureStreamNodeIdI18n(nodeId string, provider func(string) *I18n) {
	if _, ok := nodeIdMapper[nodeId]; ok {
		return
	}
	if strings.HasPrefix(nodeId, "tool-") &&
		(strings.HasSuffix(nodeId, "-stdout") || strings.HasSuffix(nodeId, "-stderr")) {
		return
	}

	entryI, _ := i18nDynamicCache.LoadOrStore(nodeId, &i18nCacheEntry{})
	entry := entryI.(*i18nCacheEntry)
	entry.once.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Warnf("stream nodeId i18n translation panicked for %q: %v", nodeId, r)
			}
		}()

		if dbResult := getNodeIdI18nFromDB(nodeId); dbResult != nil {
			entry.result = dbResult
			return
		}
		if provider != nil {
			translated := provider(nodeId)
			if translated != nil && (translated.Zh != "" || translated.En != "") {
				entry.result = translated
				saveNodeIdI18nToDB(nodeId, translated)
			}
		}
	})
}

func NodeIdToI18n(nodeId string, isStream bool) *I18n {
	if val, ok := nodeIdMapper[nodeId]; ok {
		return val
	}

	if isStream && strings.HasPrefix(nodeId, "tool-") {
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

	if isStream {
		// Pure read path: translation is populated by EnsureStreamNodeIdI18n
		// which runs before stream events are emitted.
		if cached, ok := i18nDynamicCache.Load(nodeId); ok {
			entry := cached.(*i18nCacheEntry)
			if entry.result != nil {
				return entry.result
			}
		}
		if dbResult := getNodeIdI18nFromDB(nodeId); dbResult != nil {
			return dbResult
		}
	}

	return &I18n{
		Zh: nodeId,
		En: nodeId,
	}
}

func NodeIdAndTypeToI18n(nodeId string, eventType EventType, isStream bool) *I18n {
	if eventType == "" {
		return NodeIdToI18n(nodeId, isStream)
	}
	if val, ok := eventTypeMapper[eventType]; ok {
		return val
	}
	return NodeIdToI18n(nodeId, isStream)
}

func (i18n *I18n) I18nToYPB_I18n() *ypb.I18N {
	return &ypb.I18N{
		Zh: i18n.Zh,
		En: i18n.En,
	}
}
