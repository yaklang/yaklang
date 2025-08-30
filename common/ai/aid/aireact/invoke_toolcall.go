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

func (r *ReAct) _invokeToolCall_ReviewWrongTool(oldTool *aitool.Tool, suggestionToolName, suggestionKeyword string) (*aitool.Tool, bool, error) {
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
		return nil, true, nil
	}

	var redo bool
	var selectedTool *aitool.Tool
	var answerDirectly bool
	noUserInteract := r.config.enableUserInteract
REDO:
	redo = false
	prompt, err := r.config.promptManager.GenerateToolReSelectPrompt(noUserInteract, oldTool, tools)
	if err != nil {
		return oldTool, true, err
	}
	transErr := aicommon.CallAITransaction(r.config, prompt, r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(
				rsp.GetOutputStreamReader("call-tools", true, r.Emitter),
				"require-tool", "abandon", "ask-for-clarification")
			if err != nil {
				return err
			}
			switch action.ActionType() {
			case "ask-for-clarification":
				payloads := action.GetInvokeParams(`clarification_payload`)
				question := payloads.GetString("question")
				options := payloads.GetStringSlice("options", []string{})
				suggestion, extra, err := r.RequireUserInteract(question, options)
				if err != nil {
					answerDirectly = true
					return nil
				}
				redo = true
				r.addToTimeline(
					"user-suggestion-after-clarification",
					"Question: "+question+"\nAnswer: "+suggestion+"\n"+extra,
				)
				noUserInteract = true
				return nil
			case "require-tool":
				toolName := action.GetString("tool")
				selectedTool, err = manager.GetToolByName(toolName)
				if err != nil {
					r.addToTimeline("re-select-tool-failed", fmt.Sprintf("error searching tool[%v]: %v", toolName, err))
					return utils.Errorf("error searching tool: %v", err)
				}
				r.addToTimeline("re-select-tool", fmt.Sprintf("AI Auto Re-Selected tool: %s", toolName))
			case "abandon":
				reason := action.GetString("abandon_reason")
				r.addToTimeline(
					"re-select-tool-abandoned",
					fmt.Sprintf(
						"AI Abandoned tool selection, no tool will be used. \nReason: %v",
						reason,
					),
				)
				r.EmitInfo("AI Abandoned tool selection, no tool will be used. Reason: %v", reason)
				answerDirectly = true
				return nil
			default:
				return utils.Errorf("unknown action type: %s", action.ActionType())
			}
			return nil
		})
	if transErr != nil {
		return oldTool, true, transErr
	}

	if redo {
		noUserInteract = false
		goto REDO
	}

	if selectedTool == nil {
		return oldTool, answerDirectly, nil
	}
	return selectedTool, answerDirectly, nil
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
