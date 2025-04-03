package aid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aispec"
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
			for _, toolIns := range t.config.tools {
				if toolIns.Name == toolName {
					count++
					toolRequired = append(toolRequired, toolIns)
				}
			}
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
		case "require-tool", "finished":
			return action
		}
	}
	return "unknown"
}

func (t *aiTask) callTool(ctx *taskContext, targetTool *aitool.Tool) (result *aitool.ToolResult, action string, err error) {
	t.config.EmitInfo("start to generate tool[%v] params in task:%#v", targetTool.Name, t.Name)
	// 生成申请工具详细描述的prompt
	paramsPrompt, err := t.generateRequireToolResponsePrompt(ctx, targetTool, targetTool.Name)
	if err != nil {
		err = utils.Errorf("error generate require tool response prompt: %v", err)
		t.config.EmitError("error generate require tool response prompt: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}
	// 调用AI获取工具调用参数
	req := NewAIRequest(paramsPrompt, WithAIRequest_TaskContext(ctx))
	callParams, err := t.callAI(req)
	if err != nil || callParams == nil {
		err = utils.Errorf("error calling AI: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}
	callParamsString, _ := io.ReadAll(callParams.GetOutputStreamReader("call-tools", true, t.config))

	// extract action
	callToolAction, err := extractAction(string(callParamsString), "call-tool")
	if err != nil {
		t.config.EmitError("error extract tool params: %v", err)
		err = utils.Errorf("error extracting action params: %v", err)
		return nil, "", err
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
	if !t.config.autoAgree {
		if !ep.WaitTimeoutSeconds(60) {
			t.config.EmitInfo("user review timeout, use default action: pass")
		}
	}
	params := ep.GetParams()
	if params == nil {
		t.config.EmitError("user review params is nil, plan failed")
		return nil, "", NewNonRetryableTaskStackError(utils.Errorf("user review params is nil"))
	}
	err = t.handleToolUseReview(ctx, params)
	if err != nil {
		t.config.EmitError("error handling tool use review: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}

	t.config.EmitInfo("start to execute tool:%v ", targetTool.Name)
	toolResult, err := targetTool.InvokeWithParams(callToolParams, aitool.WithStdout(stdoutBuf), aitool.WithStderr(stderrBuf))
	if err != nil {
		err = utils.Errorf("error invoking tool: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}

	t.config.EmitInfo("start to generate and feedback tool[%v] result in task:%#v", targetTool.Name, t.Name)
	// 生成调用工具结果的prompt
	decisionPrompt, err := t.generateToolCallResponsePrompt(toolResult, ctx, targetTool)
	if err != nil {
		err = utils.Errorf("error generating tool call response prompt: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}
	// 调用AI进行下一步决策
	req = NewAIRequest(decisionPrompt, WithAIRequest_TaskContext(ctx))
	continueResult, err := t.callAI(req)
	if err != nil {
		err = utils.Errorf("error calling AI: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}
	nextResponse, err := io.ReadAll(continueResult.GetOutputStreamReader("decision", true, t.config))
	if err != nil {
		err = utils.Errorf("error reading AI response: %v", err)
		return nil, "", NewNonRetryableTaskStackError(err)
	}

	// 获取下一步决策
	action = t.getToolResultAction(string(nextResponse))
	if action != "" {
		t.config.EmitInfo("tool[%v] and next do the action: %v", targetTool.Name, action)
	}
	return toolResult, action, nil
}

// executeTask 实际执行任务并返回结果
func (t *aiTask) executeTask(ctx *taskContext) error {
	// 使用Task的内部字段，如果传入的参数为nil则使用内部字段
	actualMetadata := map[string]any{}
	if t.metadata != nil {
		actualMetadata = t.metadata
	}

	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt(t.config.tools, ctx, actualMetadata)
	if err != nil {
		return fmt.Errorf("error generating aiTask prompt: %w", err)
	}

	chatDetails := aispec.ChatDetails{
		aispec.NewUserChatDetail(prompt),
	}

	// 调用AI回调函数
	t.config.EmitPrompt("task_execute", prompt)
	req := NewAIRequest(prompt, WithAIRequest_TaskContext(ctx))
	responseReader, err := t.callAI(req)
	if err != nil {
		return fmt.Errorf("error calling AI: %w", err)
	}

	// 读取AI的响应
	responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("execute", false, t.config))
	if err != nil {
		return fmt.Errorf("error reading AI response: %w", err)
	}

	// 处理工具调用, 直到没有工具调用为止
	response := string(responseBytes)
	tempChatDetails := chatDetails.Clone()
	tempChatDetails = append(tempChatDetails, aispec.NewAIChatDetail(response))

TOOLREQUIRED:
	for {
		toolRequired := t.getToolRequired(response)
		if len(toolRequired) == 0 {
			t.config.EmitInfo("no tool required in task: %#v", t.Name)
			break
		}

		targetTool := toolRequired[0]
		result, action, err := t.callTool(ctx, targetTool)
		if err != nil {
			t.config.EmitError("error calling tool: %v", err)
			return err
		}
		t.PushToolCallResult(result)

		switch action {
		case "require-more-tool":
			t.config.EmitInfo("require more tool in task: %#v", t.Name)
			moreToolPrompt, err := t.generateTaskPrompt(t.config.tools, ctx, actualMetadata)
			if err != nil {
				log.Errorf("error generating aiTask prompt: %v", err)
				break TOOLREQUIRED
			}

			req := NewAIRequest(moreToolPrompt, WithAIRequest_TaskContext(ctx))
			responseReader, err := t.callAI(req)
			if err != nil {
				return fmt.Errorf("error calling AI: %w", err)
			}
			responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("execute", false, t.config))
			if err != nil {
				return fmt.Errorf("error reading AI response: %w", err)
			}
			response = string(responseBytes)
		case "finished":
			t.config.EmitInfo("task[%v] finished", t.Name)
			fallthrough
		default:
			callHistory, err := t.generateToolCallResultsPrompt()
			if err != nil {
				log.Errorf("error generating tool call results prompt: %v", err)
				return err
			}
			response = callHistory
			break TOOLREQUIRED
		}
	}

	chatDetails = append(chatDetails, aispec.NewAIChatDetail(response))

	t.config.EmitInfo("start to execute task-summary action")
	// 处理总结回调
	summaryPromptWellFormed, err := GenerateTaskSummaryPrompt(aispec.DetailsToString(chatDetails))
	if err != nil {
		t.config.EmitError("error generating summary prompt: %v", err)
		return fmt.Errorf("error generating summary prompt: %w", err)
	}
	req = NewAIRequest(summaryPromptWellFormed, WithAIRequest_TaskContext(ctx))
	summaryReader, err := t.callAI(req)
	if err != nil {
		t.config.EmitError("error calling summary AI: %v", err)
		return fmt.Errorf("error calling summary AI: %w", err)
	}

	summaryBytes, err := io.ReadAll(summaryReader.GetOutputStreamReader("summary", false, t.config))
	if err != nil {
		t.config.EmitError("error reading summary: %v", err)
		return fmt.Errorf("error reading summary: %w", err)
	}

	action, err := extractAction(string(summaryBytes), "summary")
	if err != nil {
		t.config.EmitError("error extracting action: %v", err)
	}

	var taskSummary = ""
	shortSummary := action.GetString("short_summary")
	if shortSummary != "" {
		taskSummary = shortSummary
	}
	longSummary := action.GetString("long_summary")
	if longSummary != "" && taskSummary == "" {
		taskSummary = longSummary
	}

	t.TaskSummary = taskSummary
	t.ShortSummary = shortSummary
	t.LongSummary = longSummary

	// start to wait for user review
	ep := t.config.epm.createEndpoint()
	ep.SetDefaultSuggestionContinue()
	t.config.EmitInfo("start to wait for user review current task")

	t.config.EmitRequireReviewForTask(t, ep.id)
	if !t.config.autoAgree {
		if !ep.WaitTimeoutSeconds(60) {
			t.config.EmitInfo("user review timeout, use default action: continue")
		}
	}

	// user review finished, find params
	reviewResult := ep.GetParams()
	t.config.EmitInfo("start to handle review task event: %v", ep.id)
	err = t.handleReviewResult(ctx, reviewResult)
	if err != nil {
		log.Warnf("error handling review result: %v", err)
	}

	return nil
}
