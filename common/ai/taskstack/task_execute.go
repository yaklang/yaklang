package taskstack

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *Task) getToolRequired(response string) []*Tool {
	var toolRequired []*Tool
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
			for _, toolIns := range t.tools {
				if toolIns.Name == toolName {
					toolRequired = append(toolRequired, toolIns)
				}
			}
		}
	}
	return toolRequired
}

func (t *Task) callTool(targetTool *Tool, ctx *TaskSystemContext) (string, error) {
	paramsPrompt, err := t.generateRequireToolResponsePrompt(ctx, targetTool, targetTool.Name)
	if err != nil {
		err = utils.Errorf("error generate require tool response prompt: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}

	callParams, err := t.AICallback(paramsPrompt)
	if err != nil || callParams == nil {
		err = utils.Errorf("error calling AI: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	callParamsString, _ := io.ReadAll(callParams)
	result, err := targetTool.InvokeWithRaw(string(callParamsString))
	if err != nil {
		err = utils.Errorf("error invoking tool: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	continuePrompt, err := t.generateToolCallResponsePrompt(result, ctx, targetTool)
	if err != nil {
		err = utils.Errorf("error generating tool call response prompt: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	continueResult, err := t.AICallback(continuePrompt)
	if err != nil {
		err = utils.Errorf("error calling AI: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	nextResponse, err := io.ReadAll(continueResult)
	if err != nil {
		err = utils.Errorf("error reading AI response: %v", err)
		return "", NewNonRetryableTaskStackError(err)
	}
	return string(nextResponse), nil
}

// executeTask 实际执行任务并返回结果
func (t *Task) executeTask(ctx *TaskSystemContext) (string, error) {
	// 使用Task的内部字段，如果传入的参数为nil则使用内部字段
	actualTools := t.tools
	if actualTools == nil && t.tools != nil {
		actualTools = t.tools
	}

	actualMetadata := map[string]any{}
	if actualMetadata == nil && t.metadata != nil {
		actualMetadata = t.metadata
	}

	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt(actualTools, ctx, actualMetadata)
	if err != nil {
		return "", fmt.Errorf("error generating task prompt: %w", err)
	}

	currentPrompt := prompt
	for {
		// 调用AI回调函数
		responseReader, err := t.AICallback(currentPrompt)
		if err != nil {
			return "", fmt.Errorf("error calling AI: %w", err)
		}

		// 读取AI的响应
		responseBytes, err := io.ReadAll(responseReader)
		if err != nil {
			return "", fmt.Errorf("error reading AI response: %w", err)
		}
		response := string(responseBytes)

		var nextResponse = response
		for {
			toolRequired := t.getToolRequired(nextResponse)
			if len(toolRequired) <= 0 {
				return nextResponse, nil
			}
			for _, targetTool := range toolRequired {
				responseRaw, err := t.callTool(targetTool, ctx)
				if err != nil {
					return "", err
				}
				nextResponse = responseRaw
			}
		}
	}
}
