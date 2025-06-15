package aid

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/segmentio/ksuid"
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
		case "continue-current-task", "finished":
			return action
		}
	}
	return "unknown"
}

func (t *aiTask) callTool(targetTool *aitool.Tool) (result *aitool.ToolResult, err error) {
	t.config.EmitInfo("start to generate tool[%v] params in task:%#v", targetTool.Name, t.Name)

	callToolId := ksuid.New().String()
	t.config.EmitToolCallStart(callToolId, targetTool)
	// tool-call with stats: generating-params -> review-params -> invoking -> done/finished
	callToolDoneOnce := new(sync.Once)

	handleResultDoneCallback := func() {
		callToolDoneOnce.Do(func() {
			t.config.EmitToolCallStatus(callToolId, "done")
			t.config.EmitToolCallDone(callToolId)
		})
	}
	handleResultUserCancel := func(reason any) {
		callToolDoneOnce.Do(func() {
			t.config.EmitToolCallStatus(callToolId, fmt.Sprintf("cancelled by reason: %v", reason))
			t.config.EmitToolCallUserCancel(callToolId)
		})
	}
	handleResultErr := func(err any) {
		callToolDoneOnce.Do(func() {
			t.config.EmitToolCallError(callToolId, err)
		})
	}

	var (
		_ = handleResultDoneCallback
		_ = handleResultUserCancel
		_ = handleResultErr
	)

	defer func() {
		handleResultDoneCallback()
	}()

	// 生成申请工具详细描述的prompt
	paramsPrompt, err := t.generateRequireToolResponsePrompt(targetTool, targetTool.Name)
	if err != nil {
		err = utils.Errorf("error generate require tool response prompt: %v", err)
		t.config.EmitError("error generate require tool response prompt: %v", err)
		handleResultErr(fmt.Sprintf("error generate require tool response prompt: %v", err))
		return nil, NewNonRetryableTaskStackError(err)
	}

	var callToolParams = t.config.MakeInvokeParams()
	// transaction for generate params
	err = t.config.callAiTransaction(paramsPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.callAI(request)
	}, func(rsp *AIResponse) error {
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
		handleResultErr(err)
		return nil, NewNonRetryableTaskStackError(err)
	}

	t.config.EmitInfo("start to invoke tool:%v 's callback function", targetTool.Name)
	// DANGER: 这个值永远不应该暴露给用户，只有内部工具才有资格设置它
	if targetTool.NoNeedUserReview {
		t.config.EmitInfo("tool[%v] (internal helper tool) no need user review, skip review", targetTool.Name)
	} else {
		t.config.EmitInfo("start to require review for tool use")
		ep := t.config.epm.createEndpointWithEventType(EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE)
		ep.SetDefaultSuggestionContinue()
		t.config.EmitRequireReviewForToolUse(targetTool, callToolParams, ep.id)
		t.config.doWaitAgree(nil, ep)
		params := ep.GetParams()
		t.config.ReleaseInteractiveEvent(ep.id, params)
		if params == nil {
			t.config.EmitError("user review params is nil, tool use failed")
			handleResultErr("user review params is nil, tool use failed")
			return nil, NewNonRetryableTaskStackError(utils.Errorf("user review params is nil"))
		}
		var overrideResult *aitool.ToolResult
		var next HandleToolUseNext
		targetTool, callToolParams, overrideResult, next, err = t.handleToolUseReview(
			targetTool, callToolParams, params, handleResultUserCancel,
		)
		if err != nil {
			t.config.EmitError("error handling tool use review: %v", err)
			handleResultErr(fmt.Sprintf("error handling tool use review: %v", err))
			return nil, NewNonRetryableTaskStackError(err)
		}
		switch next {
		case HandleToolUseNext_Override:
			return overrideResult, nil
		default:
		}
	}

	// 调用工具
	stdoutReader, stdoutWriter := utils.NewPipe()
	defer stdoutWriter.Close()
	stderrReader, stderrWriter := utils.NewPipe()
	defer stderrWriter.Close()

	t.config.EmitToolCallStd(targetTool.Name, stdoutReader, stderrReader, t.Index)
	t.config.EmitInfo("start to execute tool:%v", targetTool.Name)
	toolResult, err := targetTool.InvokeWithParams(callToolParams, t.config.toolCallOpts(stdoutWriter, stderrWriter)...)
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

	var action *Action
	err = t.config.callAiTransaction(decisionPrompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.callAI(request)
	}, func(continueResult *AIResponse) error {
		nextResponse, err := io.ReadAll(continueResult.GetOutputStreamReader("decision", true, t.config))
		if err != nil {
			err = utils.Errorf("error reading AI response: %v", err)
			return utils.Errorf("error reading AI response: %v", err)
		}

		// 获取下一步决策
		action, err = ExtractAction(string(nextResponse), taskContinue, taskProceedNext, taskSkipped, taskFailed)
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

		t.config.EmitInfo("tool[%v] and next do the action: %v", targetTool.Name, action.Name())
		return nil
	})
	if err != nil {
		t.config.EmitWarning("no action found, using default action, finished")
		return "", NewNonRetryableTaskStackError(err)
	}
	return action.Name(), nil
}
