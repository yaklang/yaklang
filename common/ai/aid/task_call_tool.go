package aid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *aiTask) getToolRequired(response string) []*aitool.Tool {
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
			toolIns, err := t.config.aiToolManager.GetToolByName(toolName)
			if err != nil {
				t.config.EmitError("error searching tool: %v", err)
				continue
			}
			count++
			toolRequired = append(toolRequired, toolIns)
			if count <= 0 {
				t.config.EmitInfo("require-tool for %v, but not found it", toolName)
			}
		}
	}
	return toolRequired
}

func (t *aiTask) getToolResultAction(response string) string {
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		toolRequiredJSON := response[start:end]
		gjsonResult := gjson.Parse(toolRequiredJSON)
		action := gjsonResult.Get("@action").String()
		switch action {
		case "require-more-tool", "finished":
			return action
		}
	}
	return "unknown"
}

func (t *aiTask) callTool(targetTool *aitool.Tool) (result *aitool.ToolResult, err error) {
	t.config.EmitInfo("start to generate tool[%v] params in task:%#v", targetTool.Name, t.Name)
	// 生成申请工具详细描述的prompt
	paramsPrompt, err := t.generateRequireToolResponsePrompt(targetTool, targetTool.Name)
	if err != nil {
		err = utils.Errorf("error generate require tool response prompt: %v", err)
		t.config.EmitError("error generate require tool response prompt: %v", err)
		return nil, NewNonRetryableTaskStackError(err)
	}

	var callToolParams aitool.InvokeParams = make(aitool.InvokeParams)
	// transaction for generate params
	err = t.config.callAiTransaction(paramsPrompt, t.callAI, func(rsp *AIResponse) error {
		callParamsString, _ := io.ReadAll(rsp.GetOutputStreamReader("call-tools", true, t.config))

		// extract action
		callToolAction, err := ExtractAction(string(callParamsString), "call-tool")
		if err != nil {
			t.config.EmitError("error extract tool params: %v", err)
			err = utils.Errorf("error extracting action params: %v", err)
			return err
		}
		callToolParams = callToolAction.GetInvokeParams("params")
		return nil
	})
	if err != nil {
		err = utils.Errorf("calling AI transaction failed: %v", err)
		t.config.EmitError("critical err: %v", err)
		return nil, NewNonRetryableTaskStackError(err)
	}

	t.config.EmitInfo("start to invoke tool:%v 's callback function", targetTool.Name)
	// 调用工具
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	t.config.EmitToolCallStd(targetTool.Name, stdoutBuf, stderrBuf)

	// DANGER: 这个值永远不应该暴露给用户，只有内部工具才有资格设置它
	if targetTool.NoNeedUserReview {
		t.config.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", targetTool.Name)
	} else {
		t.config.EmitInfo("start to require review for tool use")
		ep := t.config.epm.createEndpoint()
		ep.SetDefaultSuggestionContinue()
		t.config.EmitRequireReviewForToolUse(targetTool, callToolParams, ep.id)
		t.config.doWaitAgree(nil, ep)
		params := ep.GetParams()
		t.config.ReleaseInteractiveEvent(ep.id, params)
		if params == nil {
			t.config.EmitError("user review params is nil, plan failed")
			return nil, NewNonRetryableTaskStackError(utils.Errorf("user review params is nil"))
		}
		targetTool, callToolParams, err = t.handleToolUseReview(targetTool, callToolParams, params)
		if err != nil {
			t.config.EmitError("error handling tool use review: %v", err)
			return nil, NewNonRetryableTaskStackError(err)
		}
	}
	t.config.EmitInfo("start to execute tool:%v", targetTool.Name)
	toolResult, err := targetTool.InvokeWithParams(callToolParams, t.config.toolCallOpts(stdoutBuf, stderrBuf)...)
	if err != nil {
		toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", targetTool.Name, err)
		toolResult.Success = false
	}

	t.config.EmitInfo("start to generate and feedback tool[%v] result in task:%#v", targetTool.Name, t.Name)
	// 生成调用工具结果的prompt

	return toolResult, nil
}

func (t *aiTask) toolResultDecision(result *aitool.ToolResult, targetTool *aitool.Tool) (string, error) {
	decisionPrompt, err := t.generateToolCallResponsePrompt(result, targetTool)
	if err != nil {
		err = utils.Errorf("error generating tool call response prompt: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}

	var actionFinal string
	err = t.config.callAiTransaction(decisionPrompt, t.callAI, func(continueResult *AIResponse) error {
		nextResponse, err := io.ReadAll(continueResult.GetOutputStreamReader("decision", true, t.config))
		if err != nil {
			err = utils.Errorf("error reading AI response: %v", err)
			return utils.Errorf("error reading AI response: %v", err)
		}

		// 获取下一步决策
		action, err := ExtractAction(string(nextResponse), "require-more-tool", "finished")
		if err != nil {
			return utils.Errorf("error extracting action: %v", err)
		}
		actionFinal = action.Name()
		if actionFinal != "require-more-tool" && actionFinal != "finished" {
			return utils.Errorf("error extracting action: %v", actionFinal)
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

		if t.ShortSummary != "" {
			t.TaskSummary = t.ShortSummary
		}
		if t.LongSummary != "" && t.TaskSummary == "" {
			t.TaskSummary = t.LongSummary
		}

		t.config.EmitInfo("tool[%v] and next do the action: %v", targetTool.Name, actionFinal)
		return nil
	})
	if err != nil {
		t.config.EmitWarning("no action found, using default action, finished")
		return "", NewNonRetryableTaskStackError(err)
	}
	return actionFinal, nil
}
