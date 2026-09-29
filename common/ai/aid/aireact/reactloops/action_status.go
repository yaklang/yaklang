package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// These names are for the user's activity indicator. ActionType remains the
// stable protocol identifier and is kept in the status detail for diagnostics.
type actionStatusName struct{ zh, en string }

var actionStatusNames = map[string]actionStatusName{
	"adjust_todolist":                        {"调整待办事项", "updating the task list"},
	"directly_answer":                        {"回复用户", "answering the user"},
	"finish":                                 {"完成任务", "finishing the task"},
	schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL: {"申请工具使用", "requesting a tool"},
	schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL:        {"调用工具", "calling a tool"},
	schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE:              {"批量执行工具", "running a tool batch"},
	schema.AI_REACT_LOOP_ACTION_SEARCH_CAPABILITIES:       {"查找可用能力", "finding available capabilities"},
	schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION:     {"确认需求", "clarifying the request"},
	schema.AI_REACT_LOOP_ACTION_KNOWLEDGE_ENHANCE:         {"检索知识", "searching knowledge"},
	schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT:      {"申请 AI 工作流", "requesting an AI blueprint"},
	schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN:              {"制定计划", "drafting a plan"},
	schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION:    {"执行计划", "executing the plan"},
	schema.AI_REACT_LOOP_ACTION_HTTP_FLOW_ANALYZE:         {"分析 HTTP 流量", "analyzing HTTP traffic"},
	schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS:            {"加载技能", "loading skills"},
	schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET:  {"查看技能内容", "browsing skill content"},
	schema.AI_REACT_LOOP_ACTION_LOAD_SKILL_RESOURCES:      {"加载技能资源", "loading skill resources"},
	schema.AI_REACT_LOOP_ACTION_LOAD_CAPABILITY:           {"加载能力", "loading a capability"},
	schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE:             {"保存证据", "saving evidence"},
	schema.AI_REACT_LOOP_ACTION_QUERY_MCP_SERVERS:         {"查询 MCP 服务", "querying MCP servers"},
	schema.AI_REACT_LOOP_ACTION_QUERY_MCP_TOOLS:           {"查询 MCP 工具", "querying MCP tools"},
	schema.AI_REACT_LOOP_ACTION_LIST_ASYNC_TASKS:          {"查看后台任务", "listing background tasks"},
	schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS: {"分派子任务", "dispatching subtasks"},
}

func statusNameForAction(name string) actionStatusName {
	if label, ok := actionStatusNames[name]; ok {
		return label
	}
	return actionStatusName{zh: fmt.Sprintf("执行「%s」动作", name), en: fmt.Sprintf("running the %s action", name)}
}

func actionStatusText(name string) (zh, en string) {
	label := statusNameForAction(name)
	return "正在" + label.zh, "Currently " + label.en
}

func preparingActionStatusText(name string) (zh, en string) {
	label := statusNameForAction(name)
	switch name {
	case schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL:
		return "正在申请工具使用", "Requesting tool access"
	case schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL:
		return "正在准备调用工具", "Preparing to call a tool"
	case schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE:
		return "正在准备批量执行工具", "Preparing a tool batch"
	default:
		return "正在准备" + label.zh, "Preparing: " + label.en
	}
}

func actionBatchStatusNames(names []string) (zh, en string) {
	const visibleLimit = 3
	zhNames, enNames := make([]string, 0, visibleLimit), make([]string, 0, visibleLimit)
	for _, name := range names {
		if len(zhNames) == visibleLimit {
			break
		}
		label := statusNameForAction(name)
		zhNames, enNames = append(zhNames, label.zh), append(enNames, label.en)
	}
	zh, en = strings.Join(zhNames, "、"), strings.Join(enNames, ", ")
	if remaining := len(names) - len(zhNames); remaining > 0 {
		zh += fmt.Sprintf("等 %d 个动作", remaining)
		en += fmt.Sprintf(" and %d more actions", remaining)
	}
	return zh, en
}
