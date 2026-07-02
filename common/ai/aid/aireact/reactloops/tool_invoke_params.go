package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// ToolParamAITagNames returns tool schema param names that may be carried via TOOL_PARAM_* AITAG.
func ToolParamAITagNames(tool *aitool.Tool) []string {
	if tool == nil || tool.Tool == nil || tool.InputSchema.Properties == nil {
		return nil
	}
	var names []string
	tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
		names = append(names, name)
		return true
	})
	return aicommon.FilterSupportedToolParamAITagNames(names)
}

func isLoopActionToolInvokeMetaKey(key string) bool {
	switch key {
	case "@action", "identifier", "human_readable_thought", "next_movements",
		aicommon.ReservedKeyCallExpectations, aicommon.ReservedKeyIdentifier:
		return true
	}
	if strings.HasPrefix(key, "next_movements.") {
		return true
	}
	if strings.HasPrefix(key, aicommon.GetToolParamAITagActionKey("")) {
		return true
	}
	return false
}

func sanitizeLoopToolInvokeParams(params aitool.InvokeParams) aitool.InvokeParams {
	if len(params) == 0 {
		return params
	}
	cleaned := make(aitool.InvokeParams, len(params))
	for k, v := range params {
		if isLoopActionToolInvokeMetaKey(k) {
			continue
		}
		cleaned[k] = v
	}
	return cleaned
}

// MergeLoopActionToolParams merges TOOL_PARAM_* AITAG block fields into invoke params
// and strips loop-only metadata that must not be passed to the underlying tool.
func MergeLoopActionToolParams(action *aicommon.Action, baseParams aitool.InvokeParams, aitagParamNames []string) aitool.InvokeParams {
	if baseParams == nil {
		baseParams = make(aitool.InvokeParams)
	} else {
		copied := make(aitool.InvokeParams, len(baseParams))
		for k, v := range baseParams {
			copied[k] = v
		}
		baseParams = copied
	}
	aicommon.MergeActionAITagParams(action, baseParams, aitagParamNames)
	return sanitizeLoopToolInvokeParams(baseParams)
}

// extractLoopActionBaseToolParams pulls tool arguments from an action before AITAG merge.
// Handles multiple AI response formats:
//  1. Standard: {"@action": "call-tool", "tool": "xxx", "params": {...}}
//  2. No @action: {"tool": "xxx", "params": {...}}
//  3. No tool field: {"@action": "xxx", "params": {...}}
//  4. Simplified: {"@action": "xxx", param1: val1, ...}
//  5. Direct params: {"tool": "xxx", param1: val1, ...}
func extractLoopActionBaseToolParams(action *aicommon.Action, tool *aitool.Tool) aitool.InvokeParams {
	if action == nil {
		return make(aitool.InvokeParams)
	}

	allParams := action.GetParams()
	if len(allParams) > 0 {
		if paramsField, hasParams := allParams["params"]; hasParams {
			if paramsMap, ok := paramsField.(map[string]any); ok {
				return paramsMap
			}
		}

		cleanParams := make(aitool.InvokeParams)
		metadataFields := map[string]bool{
			"@action":           true,
			"tool":              true,
			"params":            true,
			"call_expectations": true,
			"identifier":        true,
		}
		for key, value := range allParams {
			if !metadataFields[key] {
				cleanParams[key] = value
			}
		}
		if len(cleanParams) > 0 {
			return cleanParams
		}
	}

	invokeParams := action.GetInvokeParams(tool.GetName())
	if len(invokeParams) > 0 {
		return invokeParams
	}

	invokeParams = action.GetInvokeParams("next_action").GetObject(tool.GetName())
	if len(invokeParams) > 0 {
		return invokeParams
	}

	if action.ActionType() == tool.GetName() {
		invokeParams = make(aitool.InvokeParams)
		for _, paramName := range tool.Tool.InputSchema.Properties.Keys() {
			paramValue := action.GetInvokeParams(paramName)
			if len(paramValue) > 0 {
				invokeParams[paramName] = paramValue
				continue
			}
			if strVal := action.GetAnyToString(paramName); strVal != "" {
				invokeParams[paramName] = strVal
				continue
			}
			if floatVal := action.GetFloat(paramName); floatVal != 0 {
				invokeParams[paramName] = floatVal
				continue
			}
			invokeParams[paramName] = action.GetBool(paramName)
		}
	}

	return invokeParams
}

// BuildLoopActionToolInvokeParams builds final invoke params for a loop action backed by an AI tool.
// It extracts JSON params, merges TOOL_PARAM_* AITAG blocks, strips loop metadata, and
// preserves reserved keys used by ToolCaller.
func BuildLoopActionToolInvokeParams(action *aicommon.Action, tool *aitool.Tool) aitool.InvokeParams {
	baseParams := extractLoopActionBaseToolParams(action, tool)
	invokeParams := MergeLoopActionToolParams(action, baseParams, ToolParamAITagNames(tool))

	if callExpectations := action.GetString("call_expectations"); callExpectations != "" {
		invokeParams[aicommon.ReservedKeyCallExpectations] = callExpectations
	}
	if identifier := action.GetString("identifier"); identifier != "" {
		invokeParams[aicommon.ReservedKeyIdentifier] = identifier
	}
	return invokeParams
}

// ToolInvokeGuard blocks disallowed tool invocations for the active loop.
// params may be nil when the guard runs before parameters are known (e.g. require_tool).
// Return allow=false and a feedback message that will be surfaced to the AI.
type ToolInvokeGuard func(toolName string, params aitool.InvokeParams) (allow bool, feedback string)

// ToolInvokeParamsMutator adjusts tool invocation params immediately before execution.
type ToolInvokeParamsMutator func(toolName string, params aitool.InvokeParams) aitool.InvokeParams

// WithToolInvokeGuard registers a callback that can veto tool execution for this loop.
func WithToolInvokeGuard(guard ToolInvokeGuard) ReActLoopOption {
	return func(r *ReActLoop) {
		if r == nil || guard == nil {
			return
		}
		r.toolInvokeGuards = append(r.toolInvokeGuards, guard)
	}
}

// CheckToolInvokeGuard runs all guards; the first veto wins.
func CheckToolInvokeGuard(loop *ReActLoop, toolName string, params aitool.InvokeParams) (allow bool, feedback string) {
	if loop == nil || len(loop.toolInvokeGuards) == 0 {
		return true, ""
	}
	for _, guard := range loop.toolInvokeGuards {
		if guard == nil {
			continue
		}
		if ok, msg := guard(toolName, params); !ok {
			return false, msg
		}
	}
	return true, ""
}

// WithToolInvokeParamsMutator registers a callback that runs for every tool invocation
// made through the ReAct invoker while this loop is active (require_tool, directly_call_tool, etc.).
func WithToolInvokeParamsMutator(mutator ToolInvokeParamsMutator) ReActLoopOption {
	return func(r *ReActLoop) {
		if r == nil || mutator == nil {
			return
		}
		r.toolInvokeParamsMutators = append(r.toolInvokeParamsMutators, mutator)
	}
}

// ApplyToolInvokeParamsMutators runs all mutators registered on the loop.
func ApplyToolInvokeParamsMutators(loop *ReActLoop, toolName string, params aitool.InvokeParams) aitool.InvokeParams {
	if loop == nil || len(loop.toolInvokeParamsMutators) == 0 {
		return params
	}
	out := params
	for _, mutator := range loop.toolInvokeParamsMutators {
		if mutator == nil {
			continue
		}
		out = mutator(toolName, out)
	}
	return out
}
