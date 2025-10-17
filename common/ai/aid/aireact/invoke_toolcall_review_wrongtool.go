package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) _invokeToolCall_ReviewWrongTool(oldTool *aitool.Tool, suggestionToolName, suggestionKeyword string) (*aitool.Tool, bool, error) {
	manager := r.config.aiToolManager

	var tools []*aitool.Tool
	if suggestionToolName != "" {
		r.AddToTimeline("User Suggested Tools: %s", suggestionToolName)
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
		r.AddToTimeline("User Suggested Tool Keywords: %s", suggestionKeyword)
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
		r.AddToTimeline("re-select-tool", "No tools available for selection, no enabled tools, skip tool re-selection.")
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
				r.AddToTimeline(
					"user-suggestion-after-clarification",
					"Question: "+question+"\nAnswer: "+suggestion+"\n"+extra,
				)
				noUserInteract = true
				return nil
			case "require-tool":
				toolName := action.GetString("tool")
				selectedTool, err = manager.GetToolByName(toolName)
				if err != nil {
					r.AddToTimeline("re-select-tool-failed", fmt.Sprintf("error searching tool[%v]: %v", toolName, err))
					return utils.Errorf("error searching tool: %v", err)
				}
				r.AddToTimeline("re-select-tool", fmt.Sprintf("AI Auto Re-Selected tool: %s", toolName))
			case "abandon":
				reason := action.GetString("abandon_reason")
				r.AddToTimeline(
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
