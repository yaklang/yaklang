package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) _invokeToolCall_ReviewWrongParam(tool *aitool.Tool, old aitool.InvokeParams, extraPrompt string) (aitool.InvokeParams, error) {
	return nil, nil
}

func (r *ReAct) _invokeToolCall_ReviewWrongTool(oldTool *aitool.Tool, suggestionToolName, suggestionKeyword string) (*aitool.Tool, error) {
	manager := r.config.aiToolManager

	var tools []*aitool.Tool
	if suggestionToolName != "" {
		r.addToTimeline("User Suggested Tools: %s", suggestionToolName)
		log.Infof("User Suggested Tools: %s", suggestionToolName)
		for _, item := range utils.PrettifyListFromStringSplited(suggestionToolName, ",") {
			toolins, err := manager.GetToolByName(item)
			if err != nil || utils.IsNil(toolins) {
				if err != nil {
					r.EmitError("error searching tool: %v", err)
				} else {
					r.EmitInfo("suggestion tool: %v but not found it.", suggestionToolName)
				}
			}
			tools = append(tools, toolins)
		}
	}

	var err error
	if suggestionKeyword != "" {
		r.addToTimeline("User Suggested Tool Keywords: %s", suggestionKeyword)
		searched, err := manager.SearchTools("", suggestionKeyword)
		if err != nil {
			r.EmitError("error searching tool: %v", err)
		}
		tools = append(tools, searched...)
	}

	if len(tools) <= 0 {
		tools, _ = manager.GetEnableTools()
	}

	if len(tools) <= 0 {
		r.addToTimeline("re-select-tool", "No tools available for selection, no enabled tools, skip tool re-selection.")
		return nil, utils.Error("tool not found or no tools allowed next")
	}

	prompt, err := r.config.promptManager.GenerateToolReSelectPrompt(oldTool, tools)
	if err != nil {
		return oldTool, err
	}

	var selecteddTool *aitool.Tool
	transErr := aicommon.CallAITransaction(r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(
				rsp.GetOutputStreamReader("call-tools", true, r.Emitter),
				"require-tool", "abandon")
			if err != nil {
				return err
			}
			switch action.ActionType() {
			case "require-tool":
				toolName := action.GetString("tool")
				selecteddTool, err = manager.GetToolByName(toolName)
				if err != nil {
					return utils.Errorf("error searching tool: %v", err)
				}
			case "abandon":
				return nil
			default:
				return utils.Errorf("unknown action type: %s", action.ActionType())
			}
			return nil
		})
	if transErr != nil {
		return oldTool, transErr
	}
	if selecteddTool == nil {
		return oldTool, nil
	}
	return selecteddTool, nil
}

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
		aicommon.WithToolCaller_ReviewWrongTool(r._invokeToolCall_ReviewWrongTool),
		aicommon.WithToolCaller_ReviewWrongParam(r._invokeToolCall_ReviewWrongParam),
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
	prompt, err := r.config.promptManager.GenerateToolParamsPrompt(tool)
	if err != nil {
		return "", fmt.Errorf("failed to generate tool params prompt: %w", err)
	}

	if r.config.debugPrompt {
		log.Infof("Tool params prompt: %s", prompt)
	}

	return prompt, nil
}
