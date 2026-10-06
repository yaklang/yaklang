package loopinfra

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const actionStateToolSchemaNames = "native_tool_schema_names"

var nativeToolSchemaLoadAction = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "只加载业务工具完整参数 Schema 到 Timeline，不生成参数、不执行工具。" +
		"单个名称用 tool_require_payload，多个名称用 tool_require_calls。下一轮读取 CACHE_TOOL_CALL，按真实字段自行构造参数并调用 directly_call_tool。" +
		"原生函数名是 require_tool，业务工具名称只能写在参数中；已有完整 Schema 时直接复用，不重复加载。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("tool_require_payload", aitool.WithParam_Description("要加载定义的业务工具名称；与 tool_require_calls 二选一。")),
		aitool.WithStructArrayParam("tool_require_calls", []aitool.PropertyOption{
			aitool.WithParam_Description("批量加载 Schema，每项只指定业务工具，不执行业务操作；与 tool_require_payload 二选一。"),
			aitool.WithParam_Raw("minItems", 1), aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		}, nil,
			aitool.WithStringParam("tool_name", aitool.WithParam_Required(true), aitool.WithParam_Description("要加载定义的业务工具准确名称。")),
		),
	},
	ActionVerifier: verifyToolSchemaLoad,
	ActionHandler:  loadToolSchemas,
}

func verifyToolSchemaLoad(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, nil)
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return err
	}
	raw, single := lookupCanonicalActionParam(action, "tool_require_payload")
	items, batch, err := parseCanonicalBatchItems(action, requireToolBatchField)
	if err != nil {
		return err
	}
	if single == batch {
		return utils.Error("require_tool loads schemas only: provide either tool_require_payload or tool_require_calls")
	}
	if hasAnyCanonicalActionParam(action,
		directlyCallToolBatchField, "directly_call_tool_name", "directly_call_tool_params",
		"directly_call_identifier", "directly_call_expectations", "directly_call_reason",
	) {
		return utils.Error("require_tool cannot be combined with directly_call_tool fields")
	}
	var names []string
	if single {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return utils.Error("tool_require_payload must be a non-empty tool name")
		}
		names = append(names, strings.TrimSpace(name))
	} else {
		if len(items) == 0 || len(items) > toolBatchMaxCalls(loop) {
			return utils.Errorf("tool_require_calls must contain 1-%d tools to load", toolBatchMaxCalls(loop))
		}
		for _, item := range items {
			// Old optional labels are harmless: accept them without giving them
			// execution semantics. Never accept inline execution parameters here.
			if err := rejectUnknownBatchFields(item, map[string]struct{}{"tool_name": {}, "identifier": {}, "reason": {}}); err != nil {
				return err
			}
			name, err := strictBatchString(item, "tool_name", true)
			if err != nil {
				return err
			}
			for _, field := range []string{"identifier", "reason"} {
				if _, err := strictBatchString(item, field, false); err != nil {
					return err
				}
			}
			names = append(names, name)
		}
	}
	seen := make(map[string]bool)
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if !seen[name] {
			unique = append(unique, name)
			seen[name] = true
			reactloops.MaybeWarnBashBeforeEdit(loop, name)
		}
	}
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, unique)
	return nil
}

func loadToolSchemas(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	if err := verifyToolSchemaLoad(loop, action); err != nil {
		operator.Feedback("Tool schema loading rejected: " + err.Error())
		operator.Continue()
		return
	}
	names, _ := loop.GetActionExecutionValue(action, actionStateToolSchemaNames).([]string)
	loadToolSchemasByNames(loop, names, operator)
}

// Both loading actions use the same cache, MCP checks and Timeline feedback.
func loadToolSchemasByNames(loop *reactloops.ReActLoop, names []string, operator *reactloops.LoopActionHandlerOperator) bool {
	config := loop.GetConfig()
	if len(names) == 0 || config == nil || config.GetAiToolManager() == nil {
		operator.Feedback("Tool schema loading unavailable; no tools were executed.")
		operator.Continue()
		return false
	}
	manager := config.GetAiToolManager()
	ctx := toolBatchVerifierContext(loop)
	results := make([]map[string]string, 0, len(names))
	for _, name := range names {
		entry := map[string]string{"tool_name": name, "status": "error"}
		results = append(results, entry)
		if err := ctx.Err(); err != nil {
			entry["detail"] = err.Error()
			continue
		}
		if buildinaitools.IsMCPToolName(name) && !aicommon.IsMCPServersAllowedConfig(config) {
			entry["detail"] = "MCP tools are disabled"
			continue
		}
		tool, err := manager.GetToolByName(name)
		if err == nil && tool != nil && buildinaitools.IsMCPPendingStub(tool) {
			tool, err = buildinaitools.WaitForMCPLiveTool(ctx, manager, name, buildinaitools.MCPToolInitWaitTimeout, buildinaitools.MCPToolInitPollInterval, nil)
		}
		if err != nil || tool == nil {
			entry["detail"] = fmt.Sprintf("Tool unavailable: %v", err)
			continue
		}
		mutation := loop.RecordRecentlyUsedTool(tool)
		if mutation.Upsert == nil && mutation.Reuse == nil {
			entry["detail"] = "Schema could not be cached within the current tool-cache budget"
			continue
		}
		entry["status"] = "schema_loaded"
		entry["detail"] = fmt.Sprintf("工具 %q 的参数 Schema 已加载到 CACHE_TOOL_CALL，尚未执行。现在按 Schema 和当前任务构造完整参数，立即调用 directly_call_tool 继续完成本任务；不要重复加载，不要把加载当作任务完成或单独交付的阶段，也不要等待用户说继续。", name)
	}
	// Loading later entries can evict earlier ones. Do not report those entries
	// as available in the final cache state of this load batch.
	allLoaded := true
	for _, entry := range results {
		if entry["status"] == "schema_loaded" && !manager.IsRecentlyUsedTool(entry["tool_name"]) {
			entry["status"], entry["detail"] = "evicted", "Schema was evicted by the tool-cache budget; load a smaller set."
		}
		if entry["status"] != "schema_loaded" {
			allLoaded = false
		}
	}
	body, _ := json.Marshal(results)
	loop.GetInvoker().AddToTimeline("tool_schema_load", string(body))
	operator.Feedback(string(body))
	operator.Continue()
	return allLoaded
}
