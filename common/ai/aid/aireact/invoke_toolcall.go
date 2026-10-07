package aireact

import (
	"context"
	"strings"
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

// executeToolCallInternal validates, reviews and executes explicit arguments only.
// opt is forwarded to the ToolCaller (e.g. WithToolCaller_Reason, WithToolCaller_CallToolID).
func (r *ReAct) executeToolCallInternal(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
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
	statsSource := aicommon.StatsSourceToolDirect
	// The runtime owns the execution classification; caller options cannot override it.
	opt = append(opt, aicommon.WithToolCaller_StatsSource(statsSource))

	return r.withTaskEmitterScope(func(currentTask aicommon.AIStatefulTask) (*aitool.ToolResult, bool, error) {
		tool, err := r.resolveToolForCall(ctx, toolName)
		if err != nil {
			return nil, false, err
		}

		log.Infof("preparing tool with explicit params: %s - %s", tool.Name, tool.Description)

		// Explicit calls never install an argument-generation builder.
		toolCaller, err := r.newToolCallerForCall(ctx, currentTask, toolName, opt...)
		if err != nil {
			return nil, false, err
		}

		if currentLoop := r.GetCurrentLoop(); currentLoop != nil {
			if allow, guardMsg := reactloops.CheckToolInvokeGuard(currentLoop, toolName, params); !allow {
				return nil, false, utils.Error(guardMsg)
			}
			params = reactloops.ApplyToolInvokeParamsMutators(currentLoop, toolName, params)
		}
		result, directlyAnswer, err := toolCaller.CallToolWithExistedParams(tool, params)

		if err != nil {
			return nil, false, utils.Errorf("tool call failed: %w", err)
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
		r.EmitInfo("MCP tool %q is still connecting; waiting for remote server before tool execution...", toolName)
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

// tryFillVerboseNameForPlaceholder fills the VerboseName / VerboseNameZh /
// Description of a placeholder tool (used by DirectlyCallTool before the real
// tool is resolved by the loop-layer prepare callback) so the tool-call card
// emitted via emitStart carries a readable title.
//
// It only inspects the in-memory enabled tool list (which already contains MCP
// pending stubs loaded from DB cache, since AppendTools/EnableTool register
// them into toolsGetter). It never reads the database, never waits for a live
// MCP connection, and silently no-ops on any error — the real tool is still
// resolved later inside the prepare callback for the actual invocation.
func (r *ReAct) tryFillVerboseNameForPlaceholder(tool *aitool.Tool) {
	if tool == nil || r == nil || r.config == nil {
		return
	}
	mgr := r.config.GetAiToolManager()
	if mgr == nil {
		return
	}
	tools, err := mgr.GetEnableTools()
	if err != nil {
		return
	}
	for _, real := range tools {
		if real == nil || real.Name != tool.Name {
			continue
		}
		tool.VerboseName = real.VerboseName
		tool.VerboseNameZh = real.VerboseNameZh
		if strings.TrimSpace(tool.Description) == "" && strings.TrimSpace(real.Description) != "" {
			tool.Description = real.Description
		}
		return
	}
}

// newToolCallerForCall builds a ToolCaller with the shared options (emitter
// binding, owner-loop reconsideration and interval review). All calls supply
// explicit parameters; opt appends call-specific metadata and scheduling hooks.
func (r *ReAct) newToolCallerForCall(ctx context.Context, currentTask aicommon.AIStatefulTask, toolName string, opt ...aicommon.ToolCallerOption) (*aicommon.ToolCaller, error) {
	var toolCaller *aicommon.ToolCaller

	var toolCallerOptions []aicommon.ToolCallerOption
	toolCallerOptions = append(toolCallerOptions,
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_RuntimeId(r.config.Id),
		aicommon.WithToolCaller_Emitter(currentTask.GetEmitter()),
		// Preserve current-task context for script tools.
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
		aicommon.WithToolCaller_ReviewReconsider(func(ctx context.Context, tool *aitool.Tool, params, feedback aitool.InvokeParams) string {
			return r.reconsiderToolReviewForTask(ctx, currentTask, tool, params, feedback)
		}),
	)

	// Add interval review handler if not disabled (enabled by default)
	if !r.config.DisableIntervalReview {
		intervalHandler := r.CreateIntervalReviewHandlerForTask(currentTask)
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

// ExecuteToolRequiredAndCallWithoutRequired handles tool execution with provided
// parameters, skipping the parameter generation (require) phase. It directly calls
// the tool with the given params. opt is forwarded to the ToolCaller.
func (r *ReAct) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams, opt ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	return r.executeToolCallInternal(ctx, toolName, params, opt...)
}

// DirectlyCallTool handles a directly_call_tool action. It emits the tool-call
// card (loading) first, then reads reason/params from the streaming action and
// invokes the tool. The loop-layer prepare callback does param normalize/validate
// returns invalid proposals to the owning loop; it never generates arguments.
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

	// Direct calls only accept explicit arguments. Review corrections return to their owner.
	toolCaller, err := r.newToolCallerForCall(
		ctx,
		currentTask,
		toolName,
		aicommon.WithToolCaller_OmitResultParamsInTimeline(),
		aicommon.WithToolCaller_StatsSource(aicommon.StatsSourceToolDirect),
	)
	if err != nil {
		return nil, false, err
	}

	directlyCallTool := &aitool.Tool{Tool: &mcp.Tool{Name: toolName}}
	// The placeholder only carries the tool name. Try to fill its verbose
	// name / description from the in-memory enabled tool list so the
	// tool-call card (emitStart) shows a readable title immediately. The
	// real tool used for the actual invocation is still resolved later
	// inside the prepare callback (loop-layer resolveTool).
	r.tryFillVerboseNameForPlaceholder(directlyCallTool)
	result, directlyAnswer, err := toolCaller.DirectlyCallTool(directlyCallTool, action, prepare)
	if err != nil {
		return nil, false, utils.Errorf("tool call failed: %w", err)
	}
	return r.finalizeToolCallResult(currentTask, result, directlyAnswer)
}
