package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"text/template"
)

func (t *aiTask) execute() error {
	t.config.memory.StoreCurrentTask(t)
	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt()
	if err != nil {
		return fmt.Errorf("error generating aiTask prompt: %w", err)
	}

	// 调用AI回调函数
	t.config.EmitPrompt("task_execute", prompt)
	req := NewAIRequest(prompt)
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

TOOLREQUIRED:
	for {
		toolRequired := t.getToolRequired(response)
		if len(toolRequired) == 0 {
			t.config.EmitInfo("no tool required in task: %#v", t.Name)
			break
		}

		targetTool := toolRequired[0]
		result, err := t.callTool(targetTool)
		if err != nil {
			t.config.EmitError("error calling tool: %v", err)
			return err
		}
		result.ID = t.config.idGenerator()
		t.PushToolCallResult(result)

		action, err := t.toolResultDecision(result, targetTool)
		if err != nil {
			t.config.EmitError("error calling tool: %v", err)
			return err
		}

		switch action {
		case "require-more-tool":
			t.config.EmitInfo("require more tool in task: %#v", t.Name)
			moreToolPrompt, err := t.generateTaskPrompt()
			if err != nil {
				log.Errorf("error generating aiTask prompt: %v", err)
				break TOOLREQUIRED
			}

			req := NewAIRequest(moreToolPrompt)
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

	t.config.EmitInfo("start to execute task-summary action")
	// 处理总结回调
	summaryPromptWellFormed, err := t.GenerateTaskSummaryPrompt()
	if err != nil {
		t.config.EmitError("error generating summary prompt: %v", err)
		return fmt.Errorf("error generating summary prompt: %w", err)
	}
	req = NewAIRequest(summaryPromptWellFormed)
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

	action, err := ExtractAction(string(summaryBytes), "summary")
	if err != nil {
		t.config.EmitError("error extracting action: %v", err)
	}

	var taskSummary = ""
	var shortSummary = ""
	var longSummary = ""
	if action != nil {
		shortSummary = action.GetString("short_summary")
	}
	if shortSummary != "" {
		taskSummary = shortSummary
	}
	if action != nil {
		longSummary = action.GetString("long_summary")
	}
	if longSummary != "" && taskSummary == "" {
		taskSummary = longSummary
	}

	t.TaskSummary = taskSummary
	t.ShortSummary = shortSummary
	t.LongSummary = longSummary
	return nil
}

// executeTask 实际执行任务并返回结果
func (t *aiTask) executeTask() error {
	if err := t.execute(); err != nil {
		return err
	}
	// start to wait for user review
	ep := t.config.epm.createEndpoint()
	ep.SetDefaultSuggestionContinue()
	t.config.EmitInfo("start to wait for user review current task")

	t.config.EmitRequireReviewForTask(t, ep.id)
	t.config.doWaitAgree(nil, ep)
	// user review finished, find params
	reviewResult := ep.GetParams()
	t.config.memory.StoreInteractiveUserInput(ep.id, reviewResult)
	t.config.EmitInfo("start to handle review task event: %v", ep.id)
	err := t.handleReviewResult(reviewResult)
	if err != nil {
		log.Warnf("error handling review result: %v", err)
	}

	return nil
}

func (t *aiTask) GenerateTaskSummaryPrompt() (string, error) {
	summaryTemplate := template.Must(template.New("summary").Parse(__prompt_TaskSummary))
	var buf bytes.Buffer
	err := summaryTemplate.Execute(&buf, map[string]any{
		"Memory": t.config.memory,
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
