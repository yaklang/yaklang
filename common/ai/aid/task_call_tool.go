package aid

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *AiTask) getToolRequired(response string) []*aitool.Tool {
	var toolRequired []*aitool.Tool
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		toolRequiredJSON := response[start:end]
		var data = make(map[string]any)
		err := json.Unmarshal([]byte(toolRequiredJSON), &data)
		if err != nil {
			log.Errorf("error unmarshal tool required: %v", err)
			continue
		}
		if rawData, ok := data["@action"]; ok && fmt.Sprint(rawData) != "require-tool" {
			continue
		}
		if rawData, ok := data["tool"]; ok && fmt.Sprint(rawData) != "" {
			toolName := fmt.Sprint(rawData)
			count := 0
			toolIns, err := t.aiToolManager.GetToolByName(toolName)
			if err != nil {
				t.EmitError("error searching tool: %v", err)
				continue
			}
			count++
			toolRequired = append(toolRequired, toolIns)
			if count <= 0 {
				t.EmitInfo("require-tool for %v, but not found it", toolName)
			}
		}
	}
	return toolRequired
}

func (t *AiTask) getToolResultAction(response string) string {
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		toolRequiredJSON := response[start:end]
		gjsonResult := gjson.Parse(toolRequiredJSON)
		action := gjsonResult.Get("@action").String()
		switch action {
		case "continue-current-task", "finished":
			return action
		}
	}
	return "unknown"
}

func (t *AiTask) callTool(targetTool *aitool.Tool) (result *aitool.ToolResult, directlyAnswer bool, err error) {
	var caller *aicommon.ToolCaller
	caller, err = aicommon.NewToolCaller(
		aicommon.WithToolCaller_Task(t),
		aicommon.WithToolCaller_AICallerConfig(t),
		aicommon.WithToolCaller_Emitter(t.GetEmitter()),
		aicommon.WithToolCaller_AICaller(t),
		aicommon.WithToolCaller_GenerateToolParamsBuilder(t.generateRequireToolResponsePrompt),
		aicommon.WithToolCaller_OnStart(func(callToolId string) {
			caller.SetEmitter(t.Emitter.AssociativeAIProcess(&schema.AiProcess{
				ProcessId:   callToolId,
				ProcessType: schema.AI_Call_Tool,
			}))
		}),
		aicommon.WithToolCaller_OnEnd(func(callToolId string) {
			caller.SetEmitter(caller.GetEmitter().PopEventProcesser())
		}),
		aicommon.WithToolCaller_ReviewWrongTool(t.toolReviewPolicy_wrongTool),
		aicommon.WithToolCaller_ReviewWrongParam(t.toolReviewPolicy_wrongParam),
	)
	if err != nil {
		return nil, false, utils.Errorf("error creating tool caller: %v", err)
	}
	return caller.CallTool(targetTool)
}

func (t *AiTask) toolResultDecision(result *aitool.ToolResult, targetTool *aitool.Tool) (string, error) {
	decisionPrompt, err := t.generateToolCallResponsePrompt(result, targetTool)
	if err != nil {
		err = utils.Errorf("error generating tool call response prompt: %v", err)
		return "", err
	}

	var action *aicommon.Action
	err = t.callAiTransaction(decisionPrompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.CallAI(request)
	}, func(continueResult *aicommon.AIResponse) error {
		nextResponse, err := io.ReadAll(continueResult.GetOutputStreamReader("decision", true, t.GetEmitter()))
		if err != nil {
			err = utils.Errorf("error reading AI response: %v", err)
			return utils.Errorf("error reading AI response: %v", err)
		}

		// 获取下一步决策
		action, err = aicommon.ExtractAction(string(nextResponse), taskContinue, taskProceedNext, taskSkipped, taskFailed)
		if err != nil {
			return utils.Errorf("error extracting action: %v", err)
		}
		if ret := action.GetString("status_summary"); ret != "" {
			t.StatusSummary = ret
		}
		if ret := action.GetString("task_short_summary"); ret != "" {
			t.ShortSummary = ret
		}
		if ret := action.GetString("task_long_summary"); ret != "" {
			t.LongSummary = ret
		}
		if ret := action.GetString("shrink_similar_tool_call_result"); ret != "" {
			result.ShrinkSimilarResult = ret
		}
		if ret := action.GetString("summary_tool_call_result"); ret != "" {
			result.ShrinkResult = ret
		}

		if t.ShortSummary != "" {
			t.TaskSummary = t.ShortSummary
		}
		if t.LongSummary != "" && t.TaskSummary == "" {
			t.TaskSummary = t.LongSummary
		}

		t.EmitInfo("tool[%v] and next do the action: %v", targetTool.Name, action.Name())
		return nil
	})
	if err != nil {
		t.EmitWarning("no action found, using default action, finished")
		return "", err
	}
	return action.Name(), nil
}
