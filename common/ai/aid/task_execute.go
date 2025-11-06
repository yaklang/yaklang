package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"io"
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
	t.Memory.StoreCurrentTask(t)
	err := t.ExecuteLoopTask(schema.AI_REACT_LOOP_NAME_PE_TASK, t, reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
		t.EmitInfo("ReAct Loop iteration %d completed for task: %s, isDone: %v, reason: %v", iteration, t.Name, isDone, reason)
		if isDone {
			err := t.generateTaskSummary()
			if err != nil {
				log.Errorf("iteration task summary failed: %v", err)
			}
		} else {
			_, summary := loop.GetLastSatisfactionRecord()
			if summary != "" {
				t.StatusSummary = summary
			}
		}
	}))

	if err != nil {
		return err
	}
	if t.IsCtxDone() {
		return utils.Errorf("context is done")
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

func (t *AiTask) generateTaskSummary() error {
	summaryPromptWellFormed, err := t.GenerateTaskSummaryPrompt()
	if err != nil {
		t.EmitError("error generating summary prompt: %v", err)
		return fmt.Errorf("error generating summary prompt: %w", err)
	}

	var shortSummary, statusSummary, taskSummary, longSummary string

	err = t.CallAiTransaction(summaryPromptWellFormed, t.CallOriginalAI, func(summaryReader *aicommon.AIResponse) error { // 异步过程 使用无 id的 原始ai callback
		action, err := aicommon.ExtractValidActionFromStream(t.Ctx, summaryReader.GetUnboundStreamReader(false), "summary",
			aicommon.WithActionFieldStreamHandler(
				[]string{"status_summary", "task_short_summary", "task_long_summary"},
				func(key string, r io.Reader) {
					t.EmitDefaultStreamEvent("summary", utils.UTF8Reader(r), t.GetIndex())
				},
			))
		if err != nil {
			return fmt.Errorf("error reading summary: %w", err)
		}
		if action == nil {
			return utils.Errorf("error: summary is empty, retry it until summary finished")
		}
		statusSummary = action.GetString("status_summary")
		shortSummary = action.GetString("task_short_summary")
		longSummary = action.GetString("task_long_summary")

		if shortSummary != "" {
			taskSummary = shortSummary
		}
		if longSummary != "" && taskSummary == "" {
			taskSummary = longSummary
		}
		if shortSummary == "" && statusSummary == "" && longSummary == "" {
			return utils.Errorf("error: short summary ,stats summary ,long summary are empty, retry it until summary finished")
		}
		return nil
	})
	if statusSummary != "" {
		t.StatusSummary = statusSummary
	}
	if taskSummary != "" {
		t.TaskSummary = taskSummary
	}
	if shortSummary != "" {
		t.ShortSummary = shortSummary
	}
	if longSummary != "" {
		t.LongSummary = longSummary
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
