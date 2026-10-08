package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// These names are for the user's activity indicator. ActionType remains the
// stable protocol identifier in action records and execution events.
type actionStatusName struct{ zh, en string }

var actionStatusNames = map[string]actionStatusName{
	"adjust_todolist":                        {"调整待办事项", "updating the task list"},
	"directly_answer":                        {"回复用户", "answering the user"},
	"finish":                                 {"完成任务", "finishing the task"},
	schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL: {"加载工具定义", "loading tool schemas"},
	schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL:        {"执行工具", "running tools"},
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
	return actionStatusName{zh: "执行操作", en: "performing an action"}
}

// GetVerboseNameI18n resolves UI-only metadata for built-in and custom actions.
func (a *LoopAction) GetVerboseNameI18n() schema.I18n {
	label := statusNameForAction(a.ActionType)
	if a.VerboseNameI18n != nil {
		zh, en := strings.TrimSpace(a.VerboseNameI18n.Zh), strings.TrimSpace(a.VerboseNameI18n.En)
		if zh == a.ActionType {
			zh = ""
		}
		if en == a.ActionType {
			en = ""
		}
		if zh == "" {
			zh = en
		}
		if en == "" {
			en = zh
		}
		if zh != "" {
			label = actionStatusName{zh, en}
		}
	}
	return schema.I18n{Zh: label.zh, En: label.en}
}

func (r *ReActLoop) statusNameForAction(name string) actionStatusName {
	// Display updates must not invoke dynamic action factories.
	if r.actions != nil {
		if action, ok := r.actions.Get(name); ok && action != nil {
			label := r.actionForProtocol(action).GetVerboseNameI18n()
			return actionStatusName{label.Zh, label.En}
		}
	}
	label := statusNameForAction(name)
	if meta, ok := GetLoopMetadata(name); ok {
		if v := strings.TrimSpace(meta.VerboseNameZh); v != "" && v != name {
			label.zh = v
		}
		if v := strings.TrimSpace(meta.VerboseName); v != "" && v != name {
			label.en = v
		}
	}
	return label
}

func (r *ReActLoop) actionStatusText(name string) (string, string) {
	label := r.statusNameForAction(name)
	return "正在" + label.zh, "Currently " + label.en
}

func (r *ReActLoop) statusNameForCall(action *aicommon.Action) actionStatusName {
	label := r.statusNameForAction(action.Name())
	if action.Name() != schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		return label
	}
	var names, zhNames, enNames []string
	for _, name := range extractToolNamesFromAction(action) {
		if !containsStatusName(names, name) {
			names = append(names, name)
			display := r.StatusToolLabel(name)
			zhNames, enNames = append(zhNames, display.Zh), append(enNames, display.En)
		}
	}
	if len(names) > 0 {
		label.zh = "工具调用（" + joinedStatusNames(zhNames, false) + "）"
		label.en = "tool calls (" + joinedStatusNames(enNames, true) + ")"
	}
	return label
}

// StatusToolLabel keeps protocol identifiers out of user-visible status text.
// A one-language display name can serve both languages; missing metadata uses
// a neutral label while StatusTool.Name retains the identity for consumers.
func (r *ReActLoop) StatusToolLabel(name string) schema.I18n {
	label := schema.I18n{}
	if r != nil {
		if cfg := r.GetConfig(); cfg != nil && cfg.GetAiToolManager() != nil {
			if tool, err := cfg.GetAiToolManager().GetToolByName(name); err == nil && tool != nil {
				label.Zh, label.En = strings.TrimSpace(tool.GetVerboseNameZh()), strings.TrimSpace(tool.GetVerboseName())
			}
		}
	}
	return statusToolLabel(name, label)
}

func statusToolLabel(name string, label schema.I18n) schema.I18n {
	label.Zh, label.En = strings.TrimSpace(label.Zh), strings.TrimSpace(label.En)
	if label.Zh == name {
		label.Zh = ""
	}
	if label.En == name {
		label.En = ""
	}
	if label.Zh == "" {
		label.Zh = label.En
	}
	if label.En == "" {
		label.En = label.Zh
	}
	if label.Zh == "" {
		label = schema.I18n{Zh: "工具", En: "tool"}
	}
	return label
}
