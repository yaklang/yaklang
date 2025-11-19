package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"io"

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
	t.ContextProvider.StoreCurrentTask(t)
	taskUserInput := t.GetUserInput()
	utils.Debug(func() {
		fmt.Println("-----------------------TASK FORMATTED USER INPUT-----------------------")
		fmt.Println(taskUserInput)
		fmt.Println("-----------------------------------------------------------------------")
	})
	err := t.ExecuteLoopTask(
		schema.AI_REACT_LOOP_NAME_PE_TASK,
		t,
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
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
		}),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedback *bytes.Buffer, nonce string) (string, error) {
			reactiveData := utils.MustRenderTemplate(`
当前 Plan-Execution 模式进度信息：

<|PROGRESS_TASK_{{.Nonce}}|>
{{ .Progress }}
--- CURRENT_TASK ---
{{ .CurrentProgress }}
<|PROGRESS_TASK_END_{{ .Nonce }}|>

- 进度信息语义约定：
  1) 任务树状态约定
     - 标记含义：
       - [-] 表示该节点任务“执行中”
       - [ ] 表示该节点任务“未开始”
       - [x] 表示该节点任务“已完成”
     - 层级缩进表示父子任务关系；只处理与“当前任务”对应的子树。
  2) 当前任务边界
     - “当前任务”字段指明你唯一允许推进的任务节点。你必须严格在此节点范围内产出计划、步骤与执行说明。
     - 禁止启动、描述或完成非当前节点的兄弟/父层/子层任务，除非该节点内部明确需要的子步骤（且这些子步骤不改变其他节点状态）。
  3) 行为准则（必须遵守）
     - 不要假设或回填未在进度信息中出现的状态。
     - 不要“预完成”尚未执行的步骤；只就“当前任务”进行计划、细化与必要的状态更新建议。
     - 工具调用与继续决策次数受系统限制（见“任务次数执行信息”），你必须在该限制下规划行为，避免无效尝试。
     - 若需要外部信息或权限，先在输出中请求或声明前置条件，而非擅自推进其他任务。
  4) 只读规则（重要）
     - 进度信息对 AI 是只读的。框架会根据实际执行进度自动更新任务清单与状态。
     - 禁止 AI 主动修改、覆盖或判定任何任务节点的状态（包括但不限于从 [ ] 改为 [-]/[x]、新增/删除任务节点、调整层级）。
     - 如需表达状态变化的建议，请以“建议”形式描述，不得当作实际状态变更执行。
  5) 进度的使用方式
     - 用于理解：识别“当前任务”的上下文位置、其父任务目标与已进行的子步骤。
     - 用于计划：仅对“当前任务”制定可执行的下一步子步骤清单与完成判据（Done Criteria）。

`, map[string]interface{}{
				"Progress":        t.rootTask.Progress(),
				"CurrentProgress": t.Progress(),
				"Nonce":           nonce,
			})

			return reactiveData, nil
		}),
	)

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
	results, err := utils.RenderTemplate(__prompt_TaskSummary, map[string]any{
		"ContextProvider": t.Coordinator.ContextProvider,
	})
	if err != nil {
		return "", err
	}
	return results, nil
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
