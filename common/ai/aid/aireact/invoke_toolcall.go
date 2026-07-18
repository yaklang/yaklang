package aireact

import (
	"context"
	"fmt"
	"time"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) withTaskEmitterScope(fn func(currentTask aicommon.AIStatefulTask) (*aitool.ToolResult, bool, error)) (*aitool.ToolResult, bool, error) {
	var taskIndex string
	currentTask := r.GetCurrentTask()
	if !utils.IsNil(currentTask) {
		taskIndex = currentTask.GetIndex()
	}
	if currentTask == nil {
		currentTask = r.config.DefaultTask
	}
	processor := func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && event.TaskIndex == "" {
			event.TaskIndex = taskIndex
		}
		return event
	}
	var result *aitool.ToolResult
	var directly bool
	var err error
	run := func() {
		result, directly, err = fn(currentTask)
	}
	aicommon.WithEmitterProcessorOnTask(currentTask, processor, run)
	return result, directly, err
}

// executeToolCallInternal is the internal implementation that handles both regular and preset-parameter tool calls.
// If params is nil, it will use AI to generate parameters (require phase).
// If params is provided, it will skip the require phase and use the provided parameters directly.
// opt is forwarded to the ToolCaller (e.g. WithToolCaller_Reason, WithToolCaller_CallToolID).
func (r *ReAct) executeToolCallInternal(ctx context.Context, toolName string, params aitool.InvokeParams, skipRequire bool, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	// Check context cancellation early
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	if r.config != nil {
		r.config.RunVerificationWatchdogToolBlockingStart()
		defer r.config.RunVerificationWatchdogToolBlockingEnd()
	}

	return r.withTaskEmitterScope(func(currentTask aicommon.AIStatefulTask) (*aitool.ToolResult, bool, error) {
		tool, err := r.resolveToolForCall(ctx, toolName)
		if err != nil {
			return nil, false, err
		}

		if skipRequire {
			log.Infof("preparing tool with preset params: %s - %s", tool.Name, tool.Description)
		} else {
			log.Infof("preparing tool: %s - %s", tool.Name, tool.Description)
		}

		// Only the require path needs AI to generate params; the preset path skips it.
		toolCaller, err := r.newToolCallerForCall(ctx, currentTask, toolName, !skipRequire, opt...)
		if err != nil {
			return nil, false, err
		}

		// Call the tool with appropriate method
		var result *aitool.ToolResult
		var directlyAnswer bool
		if skipRequire {
			if currentLoop := r.GetCurrentLoop(); currentLoop != nil {
				if allow, guardMsg := reactloops.CheckToolInvokeGuard(currentLoop, toolName, params); !allow {
					return nil, false, utils.Error(guardMsg)
				}
				params = reactloops.ApplyToolInvokeParamsMutators(currentLoop, toolName, params)
			}
			// Call with preset parameters, skipping the require phase
			result, directlyAnswer, err = toolCaller.CallToolWithExistedParams(tool, true, params)
		} else {
			// Call with AI parameter generation (require phase included)
			result, directlyAnswer, err = toolCaller.CallTool(tool)
		}

		if err != nil {
			return nil, false, utils.Errorf("tool call failed: %v", err)
		}
		return r.finalizeToolCallResult(currentTask, result, directlyAnswer)
	})
}

// resolveToolForCall looks up a tool by name and, for MCP tools, waits for the
// background loader to replace the DB stub with a live tool (or timeout).
func (r *ReAct) resolveToolForCall(ctx context.Context, toolName string) (*aitool.Tool, error) {
	if buildinaitools.IsMCPToolName(toolName) && !aicommon.IsMCPServersAllowedConfig(r.config) {
		return nil, utils.Errorf("MCP tools are disabled for this runtime")
	}

	tool, err := r.config.AiToolManager.GetToolByName(toolName)
	if err != nil {
		return nil, utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	// For MCP tools, wait until the background loader replaces the DB stub with a live
	// tool (or timeout). This avoids TOOL_INITIALIZING failures right after engine start.
	if buildinaitools.IsMCPToolName(toolName) && buildinaitools.IsMCPPendingStub(tool) {
		r.EmitInfo("MCP tool %q is still connecting; waiting for remote server before tool require phase...", toolName)
		tool, err = buildinaitools.WaitForMCPLiveTool(
			ctx, r.config.AiToolManager, toolName,
			buildinaitools.MCPToolInitWaitTimeout,
			buildinaitools.MCPToolInitPollInterval,
			func(elapsed time.Duration) {
				r.EmitInfo("still waiting for MCP tool %q (elapsed %v)...", toolName, elapsed.Round(time.Second))
			},
		)
		if err != nil {
			return nil, err
		}
	}
	return tool, nil
}

// newToolCallerForCall builds a ToolCaller with the shared options (emitter
// binding, review handlers, interval review). withParamGenBuilder controls
// whether the AI param-generation builder is attached (required for the require
// path). opt is appended (e.g. WithToolCaller_Reason, WithToolCaller_CallToolID).
func (r *ReAct) newToolCallerForCall(ctx context.Context, currentTask aicommon.AIStatefulTask, toolName string, withParamGenBuilder bool, opt ...aicommon.ToolCallerOption) (*aicommon.ToolCaller, error) {
	var toolCaller *aicommon.ToolCaller

	var toolCallerOptions []aicommon.ToolCallerOption
	toolCallerOptions = append(toolCallerOptions,
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_AICaller(r.config),
		aicommon.WithToolCaller_RuntimeId(r.config.Id),
		aicommon.WithToolCaller_Emitter(currentTask.GetEmitter()),
		// r implements AIInvokeRuntime; enables lightweight (re)generation of the
		// tool-call reason: fallback when no reason was preset, and after a review
		// override (wrong_tool/wrong_params). No-op when nil.
		aicommon.WithToolCaller_InvokeRuntime(r),
	)

	// Add task context
	if currentTask != nil {
		toolCallerOptions = append(toolCallerOptions, aicommon.WithToolCaller_Task(currentTask))
	} else {
		toolCallerOptions = append(toolCallerOptions, aicommon.WithToolCaller_Task(r.config.DefaultTask))
	}

	if currentLoop := r.GetCurrentLoop(); currentLoop != nil {
		if allow, guardMsg := reactloops.CheckToolInvokeGuard(currentLoop, toolName, nil); !allow {
			return nil, utils.Error(guardMsg)
		}
		if withParamGenBuilder {
			toolCallerOptions = append(toolCallerOptions,
				aicommon.WithToolCaller_ParamAugment(func(invokeParams aitool.InvokeParams) aitool.InvokeParams {
					return reactloops.ApplyToolInvokeParamsMutators(currentLoop, toolName, invokeParams)
				}),
			)
		}
	}

	// Add callback handlers
	toolCallerOptions = append(toolCallerOptions,
		aicommon.WithToolCaller_OnStart(func(callToolId string) {
			toolCaller.SetEmitter(currentTask.GetEmitter().AssociativeAIProcess(&schema.AiProcess{
				ProcessId:   callToolId,
				ProcessType: schema.AI_Call_Tool,
			}))
		}),
		aicommon.WithToolCaller_OnEnd(func(callToolId string) {
			toolCaller.SetEmitter(toolCaller.GetEmitter().PopEventProcesser())
		}),
		aicommon.WithToolCaller_ReviewWrongTool(r._invokeToolCall_ReviewWrongTool),
		aicommon.WithToolCaller_ReviewWrongParam(r._invokeToolCall_ReviewWrongParam),
	)

	// Add interval review handler if not disabled (enabled by default)
	if !r.config.DisableIntervalReview {
		intervalHandler := r.CreateIntervalReviewHandler()
		if intervalHandler != nil {
			toolCallerOptions = append(toolCallerOptions,
				aicommon.WithToolCaller_IntervalReviewHandler(intervalHandler),
			)
			if r.config.IntervalReviewDuration > 0 {
				toolCallerOptions = append(toolCallerOptions,
					aicommon.WithToolCaller_IntervalReviewDuration(r.config.IntervalReviewDuration),
				)
			}
		}
	}

	if withParamGenBuilder {
		toolCallerOptions = append(toolCallerOptions,
			aicommon.WithToolCaller_GenerateToolParamsBuilderWithMeta(func(tool *aitool.Tool, toolName string) (*aicommon.ToolParamsPromptMeta, error) {
				return r.generateToolParamsPromptWithMeta(tool, toolName)
			}),
		)
	}

	toolCallerOptions = append(toolCallerOptions, opt...)

	var err error
	toolCaller, err = aicommon.NewToolCaller(ctx, toolCallerOptions...)
	if err != nil {
		return nil, utils.Errorf("failed to create tool caller: %v", err)
	}
	return toolCaller, nil
}

// finalizeToolCallResult stores the tool result on the task/timeline and emits
// completion info. Shared by all tool-call entry points.
func (r *ReAct) finalizeToolCallResult(currentTask aicommon.AIStatefulTask, result *aitool.ToolResult, directlyAnswer bool) (*aitool.ToolResult, bool, error) {
	if directlyAnswer {
		r.EmitInfo("AI suggests answering directly without using additional tools")
	}
	if result != nil {
		if result.GetID() <= 0 {
			result.ID = r.config.AcquireId()
		}
		// task save call tool result
		currentTask.PushToolCallResult(result)
		// Store the result in memory
		r.config.Timeline.PushToolResult(result)
		// Emit the result
		r.EmitInfo("Tool execution completed: %s", result.Name)
	}
	return result, directlyAnswer, nil
}

// ExecuteToolRequiredAndCall handles tool requirement action using aicommon.ToolCaller.
// It uses AI to generate tool parameters (require phase) before calling the tool.
// opt is forwarded to the ToolCaller (e.g. WithToolCaller_Reason to emit a reason on
// the start card, WithToolCaller_CallToolID to reuse an already-emitted card).
func (r *ReAct) ExecuteToolRequiredAndCall(ctx context.Context, toolName string, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	return r.executeToolCallInternal(ctx, toolName, nil, false, opt...)
}

// ExecuteToolRequiredAndCallWithoutRequired handles tool execution with provided
// parameters, skipping the parameter generation (require) phase. It directly calls
// the tool with the given params. opt is forwarded to the ToolCaller.
func (r *ReAct) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	return r.executeToolCallInternal(ctx, toolName, params, true, opt...)
}

// DirectlyCallTool handles a directly_call_tool action. It emits the tool-call
// card (loading) first, then reads reason/params from the streaming action and
// invokes the tool. The loop-layer prepare callback does param normalize/validate
// and may signal fallbackToRequire to reuse the same card and switch to the AI
// param-generation path. reason is read inside (from the action), not passed in.
func (r *ReAct) DirectlyCallTool(ctx context.Context, toolName string, action *aicommon.Action, prepare aicommon.DirectlyCallPrepareFunc) (*aitool.ToolResult, bool, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	if r.config != nil {
		r.config.RunVerificationWatchdogToolBlockingStart()
		defer r.config.RunVerificationWatchdogToolBlockingEnd()
	}

	var taskIndex string
	currentTask := r.GetCurrentTask()
	if !utils.IsNil(r.GetCurrentTask()) {
		taskIndex = r.GetCurrentTask().GetIndex()
	}
	if currentTask == nil {
		currentTask = r.config.DefaultTask
	}
	currentTask.SetEmitter(
		currentTask.GetEmitter().PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
			if event != nil && event.TaskIndex == "" {
				event.TaskIndex = taskIndex
			}
			return event
		}),
	)
	defer func() {
		currentTask.SetEmitter(currentTask.GetEmitter().PopEventProcesser())
	}()

	// Always attach the param-gen builder so fallbackToRequire can reuse this card
	// and switch to the AI param-generation path.
	toolCaller, err := r.newToolCallerForCall(
		ctx,
		currentTask,
		toolName,
		true,
		aicommon.WithToolCaller_OmitResultParamsInTimeline(),
	)
	if err != nil {
		return nil, false, err
	}

	directlyCallTool := &aitool.Tool{Tool: &mcp.Tool{Name: toolName}}
	result, directlyAnswer, err := toolCaller.DirectlyCallTool(directlyCallTool, action, prepare)
	if err != nil {
		return nil, false, utils.Errorf("tool call failed: %v", err)
	}
	return r.finalizeToolCallResult(currentTask, result, directlyAnswer)
}

// generateToolParamsPrompt generates the prompt for tool parameter generation
func (r *ReAct) generateToolParamsPrompt(tool *aitool.Tool, toolName string) (string, error) {
	result, err := r.generateToolParamsPromptWithMeta(tool, toolName)
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

// generateToolParamsPromptWithMeta generates the prompt for tool parameter generation with AITAG metadata
func (r *ReAct) generateToolParamsPromptWithMeta(tool *aitool.Tool, toolName string) (*aicommon.ToolParamsPromptMeta, error) {
	if tool == nil {
		return nil, fmt.Errorf("tool '%s' not found", toolName)
	}

	// Use PromptManager to generate the prompt with metadata
	promptResult, err := r.promptManager.GenerateToolParamsPromptWithMeta(tool)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tool params prompt: %w", err)
	}

	if r.config.DebugPrompt {
		log.Infof("Tool params prompt: %s", promptResult.Prompt)
	}

	return &aicommon.ToolParamsPromptMeta{
		Prompt:     promptResult.Prompt,
		Nonce:      promptResult.Nonce,
		ParamNames: promptResult.ParamNames,
	}, nil
}
