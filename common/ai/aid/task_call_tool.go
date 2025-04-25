package aid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
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
			toolIns, err := t.config.aiToolManager.SearchToolByName(toolName)
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

func (t *aiTask) toolChoose() (*aitool.Tool, error) {
	_prompt, err := t.generateTaskPrompt()
	if err != nil {
		log.Errorf("error generating aiTask prompt: %v", err)
		return nil, err
	}

	req := NewAIRequest(_prompt)
	responseReader, err := t.callAI(req)
	if err != nil {
		return nil, fmt.Errorf("error calling AI: %w", err)
	}
	responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("execute", false, t.config))
	if err != nil {
		return nil, fmt.Errorf("error reading AI response: %w", err)
	}
	response := string(responseBytes)
	toolRequired := t.getToolRequired(response)
	if len(toolRequired) == 0 {
		return nil, nil
	}
	targetTool := toolRequired[0]
	return targetTool, nil
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
	// 调用AI获取工具调用参数
	req := NewAIRequest(paramsPrompt)
	callParams, err := t.callAI(req)
	if err != nil || callParams == nil {
		err = utils.Errorf("error calling AI: %v", err)
		return nil, NewNonRetryableTaskStackError(err)
	}
	callParamsString, _ := io.ReadAll(callParams.GetOutputStreamReader("call-tools", true, t.config))

	// extract action
	callToolAction, err := ExtractAction(string(callParamsString), "call-tool")
	if err != nil {
		t.config.EmitError("error extract tool params: %v", err)
		err = utils.Errorf("error extracting action params: %v", err)
		return nil, err
	}
	callToolParams := callToolAction.GetInvokeParams("params")

	t.config.EmitInfo("start to invoke tool:%v 's callback function", targetTool.Name)
	// 调用工具
	stdoutBuf := bytes.NewBuffer(nil)
	stderrBuf := bytes.NewBuffer(nil)
	t.config.EmitStreamEvent(fmt.Sprintf("tool-%v-stdout", targetTool.Name), stdoutBuf)
	t.config.EmitStreamEvent(fmt.Sprintf("tool-%v-stderr", targetTool.Name), stderrBuf)

	t.config.EmitInfo("start to require review for tool use")
	ep := t.config.epm.createEndpoint()
	ep.SetDefaultSuggestionContinue()
	t.config.EmitRequireReviewForToolUse(targetTool, callToolParams, ep.id)
	t.config.doWaitAgree(nil, ep)
	params := ep.GetParams()
	t.config.memory.StoreInteractiveUserInput(ep.id, params)
	if params == nil {
		t.config.EmitError("user review params is nil, plan failed")
		return nil, NewNonRetryableTaskStackError(utils.Errorf("user review params is nil"))
	}
	targetTool, callToolParams, err = t.handleToolUseReview(targetTool, callToolParams, params)
	if err != nil {
		t.config.EmitError("error handling tool use review: %v", err)
		return nil, NewNonRetryableTaskStackError(err)
	}

	t.config.EmitInfo("start to execute tool:%v ", targetTool.Name)

	/*
		Execute tool finally
	*/
	resultId := t.config.AcquireId()
	cp := t.config.createToolCallCheckpoint(resultId)
	if err := t.config.submitToolCallRequestCheckpoint(cp, targetTool); err != nil {
		log.Errorf("error submitting tool call checkpoint: %v", err)
	}
	toolResult, err := targetTool.InvokeWithParams(callToolParams, aitool.WithStdout(stdoutBuf), aitool.WithStderr(stderrBuf), aitool.WithChatToAiFunc(aitool.ChatToAiFuncType(t.config.toolAICallback)))
	if err != nil {
		toolResult.Error = fmt.Sprintf("error invoking tool[%v]: %v", targetTool.Name, err)
		toolResult.Success = false
	}
	t.config.submitToolCallResponse(cp, toolResult)

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
	// 调用AI进行下一步决策
	req := NewAIRequest(decisionPrompt)
	continueResult, err := t.callAI(req)
	if err != nil {
		err = utils.Errorf("error calling AI: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	nextResponse, err := io.ReadAll(continueResult.GetOutputStreamReader("decision", true, t.config))
	if err != nil {
		err = utils.Errorf("error reading AI response: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}

	// 获取下一步决策
	action := t.getToolResultAction(string(nextResponse))
	if action != "" {
		t.config.EmitInfo("tool[%v] and next do the action: %v", targetTool.Name, action)
	}
	return action, nil
}
