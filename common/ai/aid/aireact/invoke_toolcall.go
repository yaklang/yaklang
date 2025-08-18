package aireact

import (
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// handleRequireTool handles tool requirement action using aicommon.ToolCaller
func (r *ReAct) handleRequireTool(toolName string) error {
	// Find the required tool
	tool, err := r.config.aiToolManager.GetToolByName(toolName)
	if err != nil {
		return utils.Errorf("tool '%s' not found: %v", toolName, err)
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
		return utils.Errorf("failed to create tool caller: %v", err)
	}

	// Call the tool using ToolCaller
	result, directlyAnswer, err := toolCaller.CallTool(tool)
	if err != nil {
		return utils.Errorf("tool call failed: %v", err)
	}

	// Handle the result
	if directlyAnswer {
		log.Infof("Tool caller suggests to answer directly without using tools")
		r.EmitInfo("AI suggests answering directly without using additional tools")
		return nil
	}

	if result != nil {
		// Store the result in memory
		if r.config.memory != nil {
			r.config.memory.PushToolCallResults(result)
		}

		// Log the result
		if result.Success {
			log.Infof("Tool '%s' executed successfully", toolName)
			if r.config.debugEvent {
				log.Infof("Tool result: %v", result.Data)
			}
		} else {
			log.Errorf("Tool '%s' execution failed: %s", toolName, result.Error)
		}

		// Emit the result
		r.EmitInfo("Tool execution completed: %s", result.Name)
	}

	return nil
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

// extractResponseContent extracts content from AI response (legacy method)
func (r *ReAct) extractResponseContent(resp *aicommon.AIResponse) string {
	if resp == nil {
		return ""
	}

	reader := resp.GetUnboundStreamReader(false)

	content, err := io.ReadAll(reader)
	if err != nil {
		log.Errorf("failed to read response content: %v", err)
		return ""
	}

	return string(content)
}
