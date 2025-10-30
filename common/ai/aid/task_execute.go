package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"
	"sync/atomic"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	taskContinue    = "continue-current-task"
	taskProceedNext = "proceed-next-task"
	taskFailed      = "task-failed"
	taskSkipped     = "task-skipped"
)

func (t *AiTask) execute() error {
	if t.IsCtxDone() {
		return utils.Errorf("context is done")
	}
	t.Memory.StoreCurrentTask(t)
	// 生成初始执行任务的prompt
	prompt, err := t.generateTaskPrompt()
	if err != nil {
		return fmt.Errorf("error generating aiTask prompt: %w", err)
	}

	// 调用AI回调函数
	t.EmitPrompt("task_execute", prompt)

	var response string
	var action *aicommon.Action
	var directlyAnswer string
	var directlyAnswerLong string
	err = t.CallAiTransaction(prompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.CallAI(request)
	}, func(rsp *aicommon.AIResponse) error {
		stream := rsp.GetOutputStreamReader("execute", false, t.Emitter)
		responseBytes, err := io.ReadAll(stream)
		if err != nil {
			return fmt.Errorf("error reading AI response: %w", err)
		}
		response = string(responseBytes)
		if len(response) <= 0 {
			return utils.Errorf("AI response is empty, retry it or check your AI model")
		}

		action, err = aicommon.ExtractAction(response, "direct-answer", `require-tool`)
		if err != nil {
			return utils.Errorf("error extracting @action (direct-answer/require-tool): %w， check miss \"@action\" field in object or @action bad str value", err)
		}

		if action.GetString("@action") == "direct-answer" {
			// 直接回答的情况
			directlyAnswer = action.GetString("direct_answer")
			if directlyAnswer == "" {
				return utils.Errorf("error: direct answer is empty, retry it until direct answer finished")
			}
			t.ProcessExtendedActionCallback(directlyAnswer)
			directlyAnswerLong = action.GetString("direct_answer_long")
			if directlyAnswerLong == "" {
				log.Errorf("error: direct answer long is empty, retry it until direct answer finished")
			}
			t.EmitInfo("task[%v] finished, directly answer: %v", t.Name, directlyAnswer)
		} else if action.GetString("@action") == "require-tool" {
			toolName := action.GetString("tool")
			if toolName == "" {
				return utils.Errorf("error: tool name is empty, retry it until tool name finished")
			}
		}

		return nil
	})
	if err != nil {
		return utils.Errorf("call ai transaction failed: %v", err)
	}

	// 处理工具调用, 直到没有工具调用为止
	toolCallCount := new(int64)
TOOLREQUIRED:
	for {
		toolRequired := t.getToolRequired(response)
		if len(toolRequired) == 0 {
			t.EmitInfo("no tool required in task: %#v", t.Name)
			break
		}

		atomic.AddInt64(toolCallCount, 1)

		targetTool := toolRequired[0]
		result, directlyAnswerFlag, err := t.callTool(targetTool)
		if directlyAnswerFlag {
			prompt, err := t.generateDirectAnswerPrompt()
			if err != nil {
				return fmt.Errorf("error generating aiTask prompt: %w", err)
			}
			err = t.CallAiTransaction(prompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				request.SetTaskIndex(t.Index)
				return t.CallAI(request)
			}, func(rsp *aicommon.AIResponse) error {
				responseBytes, err := io.ReadAll(rsp.GetOutputStreamReader("execute", false, t.Emitter))
				if err != nil {
					return fmt.Errorf("error reading AI response: %w", err)
				}
				response = string(responseBytes)
				if len(response) <= 0 {
					return utils.Errorf("AI response is empty, retry it or check your AI model")
				}

				action, err = aicommon.ExtractAction(response, "direct-answer")
				if err != nil {
					return utils.Errorf("error extracting @action (direct-answer): %w， check miss \"@action\" field in object or @action bad str value", err)
				}
				directlyAnswer = action.GetString("direct_answer")
				if directlyAnswer == "" {
					return utils.Errorf("error: direct answer is empty, retry it until direct answer finished")
				}
				t.ProcessExtendedActionCallback(directlyAnswer)
				directlyAnswerLong = action.GetString("direct_answer_long")
				if directlyAnswerLong == "" {
					log.Errorf("error: direct answer long is empty, retry it until direct answer finished")
				}
				t.EmitInfo("task[%v] finished, directly answer: %v", t.Name, directlyAnswer)
				return nil
			})
			break TOOLREQUIRED
		}
		if err != nil || result == nil {
			t.EmitError("error calling tool: %v with result %v:", err, result)
			return err
		}
		if !targetTool.NoNeedTimelineRecorded {
			result.ID = t.AcquireId()
			t.PushToolCallResult(result)
		}

		action, err := t.toolResultDecision(result, targetTool)
		if err != nil {
			t.EmitError("error calling tool: %v", err)
			return err
		}

		t.EmitToolCallSummary(result.ToolCallID, SelectSummary(t, result))

		switch action {
		case taskContinue:
			atomic.AddInt64(&t.TaskContinueCount, 1)
			t.EmitInfo("require more tool in task: %#v", t.Name)
			moreToolPrompt, err := t.generateTaskPrompt()
			if err != nil {
				log.Errorf("error generating aiTask prompt: %v", err)
				break TOOLREQUIRED
			}
			err = t.CallAiTransaction(moreToolPrompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				request.SetTaskIndex(t.Index)
				return t.CallAI(request)
			}, func(responseReader *aicommon.AIResponse) error {
				responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("execute", false, t.GetEmitter()))
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
		case taskProceedNext:
			t.EmitInfo("task[%v] finished", t.Name)
			break TOOLREQUIRED
		case taskFailed:
			t.EmitError("task[%v] failed", t.Name)
			break TOOLREQUIRED
		case taskSkipped:
			t.EmitInfo("task[%v] skipped, continue to next task", t.Name)
			break TOOLREQUIRED
		default:
			t.EmitError("unknown action: %v, skip tool require", action)
			break TOOLREQUIRED
		}
	}

	if directlyAnswer != "" {
		t.TaskSummary = directlyAnswer
		t.ShortSummary = directlyAnswer
		t.LongSummary = directlyAnswer
	}

	if t.TaskSummary == "" {
		var taskSummary = ""
		var shortSummary = ""
		var longSummary = ""

		t.EmitInfo("start to execute task-summary action")
		// 处理总结回调
		summaryPromptWellFormed, err := t.GenerateTaskSummaryPrompt()
		if err != nil {
			t.EmitError("error generating summary prompt: %v", err)
			return fmt.Errorf("error generating summary prompt: %w", err)
		}

		err = t.CallAiTransaction(summaryPromptWellFormed, t.CallAI, func(summaryReader *aicommon.AIResponse) error {
			summaryBytes, err := io.ReadAll(summaryReader.GetOutputStreamReader("summary", false, t.GetEmitter()))
			if err != nil {
				t.EmitError("error reading summary: %v", err)
				return fmt.Errorf("error reading summary: %w", err)
			}

			action, err := aicommon.ExtractAction(string(summaryBytes), "summary")
			if err != nil {
				t.EmitError("error extracting action: %v", err)
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

func (t *AiTask) executeTaskPushTaskIndex() error {
	// 在执行任务之前，推送事件到事件栈
	t.Emitter = t.GetEmitter().PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event.TaskIndex == "" {
			event.TaskIndex = t.Index
		}
		return event
	})
	defer func() {
		t.Emitter = t.GetEmitter().PopEventProcesser()
	}()

	// 执行实际的任务
	return t.executeTask()
}

// executeTask 实际执行任务并返回结果
func (t *AiTask) executeTask() error {
	if err := t.execute(); err != nil {
		return err
	}
	// start to wait for user review
	ep := t.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	t.EmitInfo("start to wait for user review current task")

	t.EmitRequireReviewForTask(t, ep.GetId())
	t.DoWaitAgree(t.Ctx, ep)
	// user review finished, find params
	reviewResult := ep.GetParams()
	t.ReleaseInteractiveEvent(ep.GetId(), reviewResult)
	t.EmitInfo("start to handle review task event: %v", ep.GetId())
	err := t.handleReviewResult(reviewResult)
	t.CallAfterReview(ep.GetSeq(), "请审查当前任务的执行结果", reviewResult)
	if err != nil {
		log.Warnf("error handling review result: %v", err)
	}

	return nil
}

func (t *AiTask) GenerateTaskSummaryPrompt() (string, error) {
	summaryTemplate := template.Must(template.New("summary").Parse(__prompt_TaskSummary))
	var buf bytes.Buffer
	err := summaryTemplate.Execute(&buf, map[string]any{
		"Memory": t.Memory,
	})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func SelectSummary(task *AiTask, callResult *aitool.ToolResult) string {
	if callResult.ShrinkResult != "" {
		return callResult.ShrinkResult
	}
	if callResult.ShrinkSimilarResult != "" {
		return callResult.ShrinkSimilarResult
	}
	if task.TaskSummary != "" {
		return task.TaskSummary
	}
	if task.StatusSummary != "" {
		return task.StatusSummary
	}
	return string(utils.Jsonify(callResult.Data))
}
