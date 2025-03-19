package taskstack

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aispec"
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

func (t *Task) getToolResultAction(response string) string {
	for _, pairs := range jsonextractor.ExtractObjectIndexes(response) {
		start, end := pairs[0], pairs[1]
		toolRequiredJSON := response[start:end]
		gjsonResult := gjson.Parse(toolRequiredJSON)
		action := gjsonResult.Get("@action").String()
		switch action {
		case "require-tool", "finished":
			return action
		}
	}
	return "unknown"
}

func (t *Task) callTool(ctx *TaskSystemContext, targetTool *Tool, chatDetails aispec.ChatDetails) (result, action string, err error) {
	// 生成申请工具详细描述的prompt
	paramsPrompt, err := t.generateRequireToolResponsePrompt(ctx, targetTool, targetTool.Name)
	if err != nil {
		err = utils.Errorf("error generate require tool response prompt: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	// 调用AI获取工具调用参数
	req := NewAIRequest(paramsPrompt, WithAIRequest_TaskContext(ctx))
	callParams, err := t.AICallback(req)
	if err != nil || callParams == nil {
		err = utils.Errorf("error calling AI: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	callParamsString, _ := io.ReadAll(callParams.Reader())
	// 调用工具
	toolResult, err := targetTool.InvokeWithRaw(string(callParamsString))
	if err != nil {
		err = utils.Errorf("error invoking tool: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	// 生成调用工具结果的prompt
	decisionPrompt, err := t.generateToolCallResponsePrompt(toolResult, ctx, targetTool)
	if err != nil {
		err = utils.Errorf("error generating tool call response prompt: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	// 调用AI进行下一步决策
	req = NewAIRequest(decisionPrompt, WithAIRequest_TaskContext(ctx))
	continueResult, err := t.AICallback(req)
	if err != nil {
		err = utils.Errorf("error calling AI: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	nextResponse, err := io.ReadAll(continueResult.Reader())
	if err != nil {
		err = utils.Errorf("error reading AI response: %v", err)
		return "", "", NewNonRetryableTaskStackError(err)
	}
	// 获取下一步决策
	action = t.getToolResultAction(string(nextResponse))

	// 获取工具调用结果,不包含决策部分
	index := strings.Index(decisionPrompt, "# 注意")
	if index != -1 {
		result = decisionPrompt[:index]
	} else {
		result = decisionPrompt
	}
	result = strings.TrimSpace(result)
	return result, action, nil
}

// executeTask 实际执行任务并返回结果
func (t *Task) executeTask(ctx *TaskSystemContext) (aispec.ChatDetails, error) {
	// 使用Task的内部字段，如果传入的参数为nil则使用内部字段
	actualTools := t.tools
	if actualTools == nil && t.tools != nil {
		actualTools = t.tools
	}

	actualMetadata := map[string]any{}
	if t.metadata != nil {
		actualMetadata = t.metadata
	}

	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt(actualTools, ctx, actualMetadata)
	if err != nil {
		return nil, fmt.Errorf("error generating task prompt: %w", err)
	}

	chatDetails := aispec.ChatDetails{
		aispec.NewUserChatDetail(prompt),
	}

	for {
		// 调用AI回调函数
		req := NewAIRequest(prompt, WithAIRequest_TaskContext(ctx))
		responseReader, err := t.AICallback(req)
		if err != nil {
			return nil, fmt.Errorf("error calling AI: %w", err)
		}

		// 读取AI的响应
		responseBytes, err := io.ReadAll(responseReader.Reader())
		if err != nil {
			return nil, fmt.Errorf("error reading AI response: %w", err)
		}

		// 处理工具调用, 直到没有工具调用为止
		response := string(responseBytes)
		tempChatDetails := chatDetails.Clone()
		tempChatDetails = append(tempChatDetails, aispec.NewAIChatDetail(response))

		for {
			toolRequired := t.getToolRequired(response)
			if len(toolRequired) == 0 {
				break
			}

			targetTool := toolRequired[0]
			result, action, err := t.callTool(ctx, targetTool, tempChatDetails)
			if err != nil {
				return nil, err
			}

			if action == "unknown" {
				return nil, fmt.Errorf("unknown action: %s", action)
			}
			if action == "finished" {
				response = result
				break
			}
			if action == "require-tool" {
				tempChatDetails = append(tempChatDetails,
					aispec.NewToolChatDetail(targetTool.Name, result),
					aispec.NewUserChatDetail(__prompt_REQUIRE_MORE_TOOL),
				)

				req := NewAIRequest(aispec.DetailsToString(tempChatDetails), WithAIRequest_TaskContext(ctx))
				responseReader, err := t.AICallback(req)
				if err != nil {
					return nil, fmt.Errorf("error calling AI: %w", err)
				}
				responseBytes, err := io.ReadAll(responseReader.Reader())
				if err != nil {
					return nil, fmt.Errorf("error reading AI response: %w", err)
				}
				response = string(responseBytes)
			}
		}

		chatDetails = append(chatDetails, aispec.NewAIChatDetail(response))

		// 处理响应回调, 直到没有继续思考为止
		if t.ResponseCallback != nil {
			continueThinking, newPrompt, err := t.ResponseCallback(ctx, chatDetails...)
			if err != nil {
				return nil, fmt.Errorf("error calling response callback: %w", err)
			}
			if !continueThinking {
				break
			}
			chatDetails = append(chatDetails, aispec.NewUserChatDetail(newPrompt))
		} else {
			break
		}
	}

	// 处理总结回调
	summaryCallback := t.SummaryAICallback
	if summaryCallback == nil {
		summaryCallback = t.AICallback
	}
	summaryPromptWellFormed, err := GenerateSummaryPrompt(aispec.DetailsToString(chatDetails))
	if err != nil {
		return nil, fmt.Errorf("error generating summary prompt: %w", err)
	}
	req := NewAIRequest(summaryPromptWellFormed, WithAIRequest_TaskContext(ctx))
	summaryReader, err := summaryCallback(req)
	if err != nil {
		return nil, fmt.Errorf("error calling summary AI: %w", err)
	}

	summaryBytes, err := io.ReadAll(summaryReader.Reader())
	if err != nil {
		return nil, fmt.Errorf("error reading summary: %w", err)
	}
	return aispec.ChatDetails{
		aispec.NewUserChatDetail(prompt),
		aispec.NewAIChatDetail(string(summaryBytes)),
	}, nil
}
