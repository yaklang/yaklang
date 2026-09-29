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
	Description: "Load business-tool parameter schemas into the timeline only. This does not generate arguments or execute tools. " +
		"Use tool_require_payload for one tool or tool_require_calls for several. After observing the loaded schemas in the next response, " +
		"construct arguments yourself and use directly_call_tool (single or batch). If a complete schema is already visible, call directly instead of loading it again.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("tool_require_payload", aitool.WithParam_Description("Exact tool name to load. Omit when tool_require_calls is present.")),
		aitool.WithStructArrayParam("tool_require_calls", []aitool.PropertyOption{
			aitool.WithParam_Description("Load several schemas; no business operations are executed. Each item identifies a tool, not an execution request. Mutually exclusive with tool_require_payload."),
			aitool.WithParam_Raw("minItems", 1), aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		}, nil,
			aitool.WithStringParam("tool_name", aitool.WithParam_Required(true), aitool.WithParam_Description("Exact tool name to load.")),
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
			names = append(names, name)
		}
	}
	seen := make(map[string]bool)
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if !seen[name] {
			unique = append(unique, name)
			seen[name] = true
		}
	}
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, unique)
	return nil
}

func loadToolSchemas(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	names, _ := loop.GetActionExecutionValue(action, actionStateToolSchemaNames).([]string)
	config := loop.GetConfig()
	if len(names) == 0 || config == nil || config.GetAiToolManager() == nil {
		operator.Feedback("Tool schema loading unavailable; no tools were executed.")
		operator.Continue()
		return
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
		mutation := config.RecordRecentlyUsedTool(tool)
		if mutation.Upsert == nil && mutation.Reuse == nil {
			entry["detail"] = "Schema could not be cached within the current tool-cache budget"
			continue
		}
		entry["status"] = "schema_loaded"
		entry["detail"] = "Read CACHE_TOOL_CALL in the timeline, construct arguments, then use directly_call_tool. No execution has occurred."
	}
	// Loading later entries can evict earlier ones. Do not report those entries
	// as available in the final cache state of this load batch.
	for _, entry := range results {
		if entry["status"] == "schema_loaded" && !manager.IsRecentlyUsedTool(entry["tool_name"]) {
			entry["status"], entry["detail"] = "evicted", "Schema was evicted by the tool-cache budget; load a smaller set."
		}
	}
	body, _ := json.Marshal(results)
	loop.GetInvoker().AddToTimeline("tool_schema_load", string(body))
	operator.Feedback(string(body))
	operator.Continue()
}
