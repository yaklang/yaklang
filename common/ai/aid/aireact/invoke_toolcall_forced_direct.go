package aireact

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// forcedDirectlyCallMaxRetries is the maximum number of AI attempts when
// forcing the model to call a specific tool via directly_call_tool.
const forcedDirectlyCallMaxRetries = 3

// forcedDirectlyCallSuffixTemplate is appended to the reactive-data section of
// the loop prompt to instruct the AI to call a specific tool.
const forcedDirectlyCallSuffixTemplate = `
<|FORCED_DIRECTIVE_%[1]s|>
# 强制工具调用指令 / Forced Tool Call Directive

你必须使用 directly_call_tool 调用工具「%[2]s」。
请查看 CACHE_TOOL_CALL 中该工具的 Schema，根据任务上下文构造完整参数后执行。

要求：
- @action 必须为 directly_call_tool
- directly_call_tool_name 必须为 %[2]s
- 不要使用其他工具，不要 finish，不要 directly_answer
- 如果参数不确定，参考 CACHE_TOOL_CALL 中的 Schema 定义
<|FORCED_DIRECTIVE_END_%[1]s|>
`

// executeForcedDirectlyCall implements the new 3-step flow for skipRequire=false:
//  1. Load the tool's schema into the timeline (CACHE_TOOL_CALL) so the AI can
//     see the tool's parameter definitions.
//  2. Call the AI using the loop prompt with a tail directive instructing it to
//     use directly_call_tool for the target tool.
//  3. Verify the AI's response is a directly_call_tool action targeting the
//     expected tool. If not, retry up to forcedDirectlyCallMaxRetries times.
//
// On success the captured action is passed to DirectlyCallTool for execution.
// This replaces the legacy generateParams → CallTool(false) path.
func (r *ReAct) executeForcedDirectlyCall(
	ctx context.Context,
	currentTask aicommon.AIStatefulTask,
	tool *aitool.Tool,
	toolName string,
	opt ...aicommon.ToolCallerOption,
) (*aitool.ToolResult, bool, error) {
	// Step 1: Load schema into timeline (CACHE_TOOL_CALL)
	r.config.RecordRecentlyUsedTool(tool)
	r.AddToTimeline("tool_schema_load", fmt.Sprintf(
		"Tool '%s' schema loaded into CACHE_TOOL_CALL. AI should use directly_call_tool to execute.", toolName))

	// Step 2 & 3: call AI with forced directive, retry up to maxRetries
	for attempt := 0; attempt < forcedDirectlyCallMaxRetries; attempt++ {
		capturedAction, err := r.callAIForForcedDirectlyCall(ctx, currentTask, toolName, attempt)
		if err != nil {
			log.Warnf("forced directly_call_tool attempt %d failed: %v", attempt+1, err)
			r.AddToTimeline("forced_tool_call_retry", fmt.Sprintf(
				"Attempt %d/%d failed: %v", attempt+1, forcedDirectlyCallMaxRetries, err))
			continue
		}

		// Verify the action targets the expected tool
		actualToolName := capturedAction.GetString("directly_call_tool_name")
		if actualToolName == "" {
			actualToolName = capturedAction.GetInvokeParams("next_action").GetString("directly_call_tool_name")
		}
		if actualToolName != toolName {
			log.Warnf("forced directly_call_tool attempt %d: expected %s but got %s",
				attempt+1, toolName, actualToolName)
			r.AddToTimeline("forced_tool_call_mismatch", fmt.Sprintf(
				"Attempt %d: expected tool '%s' but AI chose '%s'", attempt+1, toolName, actualToolName))
			continue
		}

		// Step 3 success: execute via DirectlyCallTool
		log.Infof("forced directly_call_tool: AI called expected tool '%s' on attempt %d", toolName, attempt+1)
		result, directly, execErr := r.executeForcedDirectlyCallAction(ctx, currentTask, toolName, capturedAction, opt...)
		if execErr != nil {
			return nil, false, utils.Errorf("forced tool call execution failed: %v", execErr)
		}
		return r.finalizeToolCallResult(currentTask, result, directly)
	}

	return nil, false, utils.Errorf("AI failed to call tool '%s' after %d attempts", toolName, forcedDirectlyCallMaxRetries)
}

// callAIForForcedDirectlyCall builds a loop prompt with a forced directive and
// calls the AI, capturing the directly_call_tool action from the response.
func (r *ReAct) callAIForForcedDirectlyCall(
	ctx context.Context,
	currentTask aicommon.AIStatefulTask,
	toolName string,
	attempt int,
) (*aicommon.Action, error) {
	n := nonce()
	forcedDirective := fmt.Sprintf(forcedDirectlyCallSuffixTemplate, n, toolName)

	userQuery := ""
	if currentTask != nil {
		userQuery = currentTask.GetUserInput()
	}

	// Build schema: use the loop's last schema if available, otherwise build a
	// minimal one with directly_call_tool so the AI knows the output format.
	var schemaStr string
	loop := promptLoopForTask(currentTask)
	if loop != nil {
		schemaStr = loop.GetLastLoopSchema()
	}
	if schemaStr == "" {
		if action, ok := reactloops.GetLoopAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL); ok {
			schemaStr = reactloops.BuildSchema(action)
		}
	}

	result, err := r.AssembleLoopPrompt(nil, &aicommon.LoopPromptAssemblyInput{
		Nonce:                   n,
		FunctionCallMode:        r.config.GetConfigBool("EnableFunctionCallMode"),
		UserQuery:               userQuery,
		Schema:                  schemaStr,
		ReactiveData:            forcedDirective,
		IncludeLatestModelReplay: false,
	})
	if err != nil {
		return nil, utils.Errorf("failed to assemble forced-directly-call prompt: %v", err)
	}
	prompt := result.Prompt

	if r.config.DebugPrompt {
		log.Infof("forced directly_call_tool prompt (attempt %d):\n%s", attempt+1, utils.ShrinkString(prompt, 500))
	}

	// Call AI and capture the action
	var capturedAction *aicommon.Action
	transErr := aicommon.CallAITransaction(r.config, prompt, r.config.CallAI, func(rsp *aicommon.AIResponse) error {
		stream := rsp.GetOutputStreamReader("call-tools", true, r.Emitter)
		action, err := aicommon.ExtractValidActionFromStream(ctx, stream, "directly_call_tool")
		if err != nil {
			return utils.Errorf("failed to extract directly_call_tool action: %v", err)
		}
		capturedAction = action
		return nil
	}, aicommon.WithAIRequest_CallerLabel("forced-directly-call"),
		aicommon.WithAIRequest_Context(ctx))
	if transErr != nil {
		return nil, transErr
	}
	if capturedAction == nil {
		return nil, utils.Error("AI response did not contain a valid directly_call_tool action")
	}
	return capturedAction, nil
}

// executeForcedDirectlyCallAction executes the captured directly_call_tool
// action via the standard DirectlyCallTool path, reusing the same ToolCaller
// machinery (card emission, review, param validation, etc).
func (r *ReAct) executeForcedDirectlyCallAction(
	ctx context.Context,
	currentTask aicommon.AIStatefulTask,
	toolName string,
	action *aicommon.Action,
	opt ...aicommon.ToolCallerOption,
) (*aitool.ToolResult, bool, error) {
	prepare := func(act *aicommon.Action, name string) (aitool.InvokeParams, bool, *aitool.Tool, error) {
		tool, err := r.resolveToolForCall(ctx, toolName)
		if err != nil {
			return nil, false, nil, utils.Errorf("tool '%s' not found: %v", toolName, err)
		}

		// Read params from the action's directly_call_tool_params field
		params := act.GetInvokeParams("directly_call_tool_params")
		if params == nil {
			params = make(aitool.InvokeParams)
		}

		// Validate params against the tool schema
		if valid, errs := tool.ValidateParams(params); !valid {
			return nil, false, tool, utils.Errorf("invalid params for '%s': %s", toolName, strings.Join(errs, "; "))
		}

		// Inject reserved keys from directly_call_ prefixed fields
		if id := act.GetString("directly_call_identifier"); id != "" {
			params[aicommon.ReservedKeyIdentifier] = id
		}
		if ce := act.GetString("directly_call_expectations"); ce != "" {
			params[aicommon.ReservedKeyCallExpectations] = ce
		}

		return params, false, tool, nil
	}

	return r.DirectlyCallTool(ctx, toolName, action, prepare)
}
