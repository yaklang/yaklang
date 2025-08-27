package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// handleRequireTool handles tool requirement action using aicommon.ToolCaller
func (r *ReAct) handleRequireTool(toolName string) (*aitool.ToolResult, bool, error) {
	// Find the required tool
	tool, err := r.config.aiToolManager.GetToolByName(toolName)
	if err != nil {
		return nil, false, utils.Errorf("tool '%s' not found: %v", toolName, err)
	}

	log.Infof("preparing tool: %s - %s", tool.Name, tool.Description)

	// Create ToolCaller with parameter generation prompt builder
	toolCaller, err := aicommon.NewToolCaller(
		aicommon.WithToolCaller_AICallerConfig(r.config),
		aicommon.WithToolCaller_AICaller(r.config),
		aicommon.WithToolCaller_Task(r.config.task),
		aicommon.WithToolCaller_RuntimeId(r.config.id),
		aicommon.WithToolCaller_Emitter(r.config.Emitter),
		aicommon.WithToolCaller_GenerateToolParamsBuilder(func(tool *aitool.Tool, toolName string) (string, error) {
			return r.generateToolParamsPrompt(tool, toolName)
		}),
	)
	if err != nil {
		return nil, false, utils.Errorf("failed to create tool caller: %v", err)
	}

	// Call the tool using ToolCaller
	result, directlyAnswer, err := toolCaller.CallTool(tool)
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
		// Store the result in memory
		r.config.memory.PushToolCallResults(result)
		// Emit the result
		r.EmitInfo("Tool execution completed: %s", result.Name)
	}
	return result, directlyAnswer, nil
}

// generateToolParamsPrompt generates the prompt for tool parameter generation
func (r *ReAct) generateToolParamsPrompt(tool *aitool.Tool, toolName string) (string, error) {
	if tool == nil {
		return "", fmt.Errorf("找不到名为 '%s' 的工具", toolName)
	}

	if r.config.memory == nil {
		return "", utils.Error("memory is not initialized")
	}

	// Use PromptManager to generate the prompt
	promptManager := NewPromptManager(r)
	prompt, err := promptManager.GenerateToolParamsPrompt(tool)
	if err != nil {
		return "", fmt.Errorf("failed to generate tool params prompt: %w", err)
	}

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", prompt)
	}

	return prompt, nil
}
