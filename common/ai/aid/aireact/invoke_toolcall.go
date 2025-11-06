package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// executeToolCallInternal is the internal implementation that handles both regular and preset-parameter tool calls.
// If params is nil, it will use AI to generate parameters (require phase).
// If params is provided, it will skip the require phase and use the provided parameters directly.
func (r *ReAct) executeToolCallInternal(ctx context.Context, toolName string, params aitool.InvokeParams, skipRequire bool) (*aitool.ToolResult, bool, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	// Check context cancellation early
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	// Setup task-aware event emitter
	var taskId string
	currentTask := r.GetCurrentTask()
	if !utils.IsNil(r.GetCurrentTask()) {
		taskId = r.GetCurrentTask().GetId()
	}
	if currentTask == nil {
		currentTask = r.config.DefaultTask
	}
	currentTask.SetEmitter(
		currentTask.GetEmitter().PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
			if event != nil && event.TaskIndex == "" {
				event.TaskIndex = taskId
			}
			return event
		}),
	)
	defer func() {
		currentTask.SetEmitter(currentTask.GetEmitter().PopEventProcesser())
	}()

	// Find the required tool
	tool, err := r.config.AiToolManager.GetToolByName(toolName)
	if err != nil {
		return nil, false, utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	if skipRequire {
		log.Infof("preparing tool with preset params: %s - %s", tool.Name, tool.Description)
	} else {
		log.Infof("preparing tool: %s - %s", tool.Name, tool.Description)
	}

	// Create ToolCaller with appropriate options
	var toolCaller *aicommon.ToolCaller

	var toolCallerOptions []aicommon.ToolCallerOption
	toolCallerOptions = append(toolCallerOptions,
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_AICaller(r.config),
		aicommon.WithToolCaller_RuntimeId(r.config.Id),
		aicommon.WithToolCaller_Emitter(currentTask.GetEmitter()),
	)

	// Add task context
	if currentTask != nil {
		toolCallerOptions = append(toolCallerOptions, aicommon.WithToolCaller_Task(currentTask))
	} else {
		toolCallerOptions = append(toolCallerOptions, aicommon.WithToolCaller_Task(r.config.DefaultTask))
	}

	// Add callback handlers
	toolCallerOptions = append(toolCallerOptions,
		aicommon.WithToolCaller_OnStart(func(callToolId string) {
			toolCaller.SetEmitter(r.config.Emitter.AssociativeAIProcess(&schema.AiProcess{
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

	// Only add parameter generation builder if we need AI to generate params
	if !skipRequire {
		toolCallerOptions = append(toolCallerOptions,
			aicommon.WithToolCaller_GenerateToolParamsBuilder(func(tool *aitool.Tool, toolName string) (string, error) {
				return r.generateToolParamsPrompt(tool, toolName)
			}),
		)
	}

	toolCaller, err = aicommon.NewToolCaller(ctx, toolCallerOptions...)
	if err != nil {
		return nil, false, utils.Errorf("failed to create tool caller: %v", err)
	}

	// Call the tool with appropriate method
	var result *aitool.ToolResult
	var directlyAnswer bool
	if skipRequire {
		// Call with preset parameters, skipping the require phase
		result, directlyAnswer, err = toolCaller.CallToolWithExistedParams(tool, true, params)
	} else {
		// Call with AI parameter generation (require phase included)
		result, directlyAnswer, err = toolCaller.CallTool(tool)
	}

	if err != nil {
		return nil, false, utils.Errorf("tool call failed: %v", err)
	}

	// Handle the result
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
func (r *ReAct) ExecuteToolRequiredAndCall(ctx context.Context, toolName string) (*aitool.ToolResult, bool, error) {
	return r.executeToolCallInternal(ctx, toolName, nil, false)
}

// ExecuteToolRequiredAndCallWithoutRequired handles tool execution with provided parameters,
// skipping the parameter generation (require) phase. It directly calls the tool with the given params.
// This is useful when parameters are already known and don't need to be generated by AI.
func (r *ReAct) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	return r.executeToolCallInternal(ctx, toolName, params, true)
}

// generateToolParamsPrompt generates the prompt for tool parameter generation
func (r *ReAct) generateToolParamsPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if tool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	// Use PromptManager to generate the prompt
	prompt, err := r.promptManager.GenerateToolParamsPrompt(tool)
	if err != nil {
		return "", fmt.Errorf("failed to generate tool params prompt: %w", err)
	}

	if r.config.DebugPrompt {
		log.Infof("Tool params prompt: %s", prompt)
	}

	return prompt, nil
}
