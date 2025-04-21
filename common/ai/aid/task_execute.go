package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

	var response string
	err = t.config.callAiTransaction(prompt, t.callAI, func(rsp *AIResponse) error {
		responseBytes, err := io.ReadAll(rsp.GetOutputStreamReader("execute", false, t.config))
		if err != nil {
			return fmt.Errorf("error reading AI response: %w", err)
		}
		response = string(responseBytes)
		if len(response) <= 0 {
			return utils.Errorf("AI response is empty, retry it or check your AI model")
		}
		return nil
	})
	if err != nil {
		return utils.Errorf("call ai transaction failed: %v", err)
	}

	// 处理工具调用, 直到没有工具调用为止
TOOLREQUIRED:
	for {
		if t.config.taskAIRespCallback != nil {
			t.config.taskAIRespCallback(response, t.config)
		}
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
		if !targetTool.NoNeedTimelineRecorded {
			result.ID = t.config.idGenerator()
			t.PushToolCallResult(result)
		}

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
			err = t.config.callAiTransaction(moreToolPrompt, t.callAI, func(responseReader *AIResponse) error {
				responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("execute", false, t.config))
				if err != nil {
					return fmt.Errorf("error reading AI response: %w", err)
				}
				response = string(responseBytes)
				if len(response) <= 0 {
					return utils.Errorf("AI response is empty, retry it or check your AI model")
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("error calling AI transaction: %w", err)
			}
			continue
		case "finished":
			t.config.EmitInfo("task[%v] finished", t.Name)
			break TOOLREQUIRED
		default:
			t.config.EmitError("unknown action: %v, skip tool require", action)
			break TOOLREQUIRED
		}
	}

	if t.TaskSummary == "" {
		var taskSummary = ""
		var shortSummary = ""
		var longSummary = ""

		t.config.EmitInfo("start to execute task-summary action")
		// 处理总结回调
		summaryPromptWellFormed, err := t.GenerateTaskSummaryPrompt()
		if err != nil {
			t.config.EmitError("error generating summary prompt: %v", err)
			return fmt.Errorf("error generating summary prompt: %w", err)
		}

		err = t.config.callAiTransaction(summaryPromptWellFormed, t.callAI, func(summaryReader *AIResponse) error {
			summaryBytes, err := io.ReadAll(summaryReader.GetOutputStreamReader("summary", false, t.config))
			if err != nil {
				t.config.EmitError("error reading summary: %v", err)
				return fmt.Errorf("error reading summary: %w", err)
			}

			action, err := ExtractAction(string(summaryBytes), "summary")
			if err != nil {
				t.config.EmitError("error extracting action: %v", err)
			}

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

			if shortSummary == "" {
				return utils.Errorf("error: short summary is empty, retry it until summary finished")
			}
			return nil
		})
		t.TaskSummary = taskSummary
		t.ShortSummary = shortSummary
		t.LongSummary = longSummary
	}
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
	t.config.ReleaseInteractiveEvent(ep.id, reviewResult)
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
