package aid

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
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

	// Emit task execution start status
	t.planLoadingStatus(fmt.Sprintf("执行子任务 [%s] / Executing Subtask [%s]: %s", t.Index, t.Index, t.Name))

	err := t.ExecuteLoopTask(
		schema.AI_REACT_LOOP_NAME_PE_TASK,
		t,
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
			t.EmitInfo("ReAct Loop iteration %d completed for task: %s, isDone: %v, reason: %v", iteration, t.Name, isDone, reason)

			// Emit iteration status
			t.planLoadingStatus(fmt.Sprintf("任务 [%s] 迭代 %d 完成 / Task [%s] Iteration %d Completed", t.Index, iteration, t.Index, iteration))

			// Log current task status for debugging - similar to prompt context format
			log.Infof("=== Post Iteration Task Status ===")
			log.Infof("Current Task Index: %s", t.Index)
			log.Infof("Current Task Name: %s", t.Name)
			log.Infof("Current Task Goal: %s", t.GetUserInput())
			log.Infof("Current Task Status: %s", t.GetStatus())
			if t.rootTask != nil {
				log.Infof("Root Task Progress:\n%s", t.rootTask.Progress())
			}
			log.Infof("=== End Task Status ===")

			// Check if completed_task_index indicates this task should be marked as done
			// This provides an additional mechanism to end tasks beyond just isDone
			lastRecord := loop.GetLastSatisfactionRecordFull()
			var summary, completedTaskIndex, nextMovements string
			if lastRecord != nil {
				summary = lastRecord.Reason
				completedTaskIndex = lastRecord.CompletedTaskIndex
				nextMovements = lastRecord.NextMovements
			}

			// Check if current task index is in the completed_task_index list
			shouldComplete := isDone
			if !shouldComplete && completedTaskIndex != "" {
				// completed_task_index can be a single index like "1-1" or multiple like "1-1,1-2"
				completedIndexes := strings.Split(completedTaskIndex, ",")
				for _, idx := range completedIndexes {
					trimmedIdx := strings.TrimSpace(idx)
					if trimmedIdx == t.Index {
						log.Infof("task %s marked as completed via completed_task_index: %s", t.Name, completedTaskIndex)
						t.EmitInfo("Task %s completed via completed_task_index mechanism", t.Name)
						shouldComplete = true
						break
					}
				}
			}

			if shouldComplete {
				// Emit task completing status
				t.planLoadingStatus(fmt.Sprintf("任务 [%s] 正在总结 / Task [%s] Generating Summary...", t.Index, t.Index))

				err := t.generateTaskSummary()
				if err != nil {
					log.Errorf("iteration task summary failed: %v", err)
					t.planLoadingStatus(fmt.Sprintf("任务 [%s] 总结失败 / Task [%s] Summary Failed", t.Index, t.Index))
				} else {
					t.planLoadingStatus(fmt.Sprintf("任务 [%s] 已完成 / Task [%s] Completed", t.Index, t.Index))
				}

				// Signal the loop to end - this ensures the loop terminates after this iteration
				operator.EndIteration("task completed via completed_task_index or isDone")
			} else {
				// Emit continuing status
				t.planLoadingStatus(fmt.Sprintf("任务 [%s] 继续执行 (迭代 %d) / Task [%s] Continuing (Iteration %d)", t.Index, iteration+1, t.Index, iteration+1))

				// Combine summary (reasoning) and next_movements as Processing status
				// This ensures both are captured in StatusSummary to avoid context loss
				t.updateProcessingStatus(summary, nextMovements)
			}
		}),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedback *bytes.Buffer, nonce string) (string, error) {
			reactiveData := utils.MustRenderTemplate(`
当前 Plan-Execution 模式进度信息：

<|PROGRESS_TASK_{{.Nonce}}|>
{{ .Progress }}

--- CURRENT_TASK ---
{{ .CurrentProgress }}
--- CURRENT_TASK_END ---

<|PROGRESS_TASK_END_{{ .Nonce }}|>

- 进度信息语义约定：
  1) 任务树状态约定
     - 标记含义：
       - [-] 表示该节点任务“执行中”
       - [ ] 表示该节点任务“未开始”
       - [x] 表示该节点任务“已完成”
     - 层级缩进表示父子任务关系；只处理与“当前任务”对应的子树。
  2) 当前任务边界
     - “当前任务(CURRENT_TASK)”指明你唯一允许推进的任务节点。你必须严格在此节点范围内产出计划、步骤与执行说明。
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
		if t.GetStatus() == aicommon.AITaskState_Skipped {
			return nil
		}
		return err
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
	if t.IsCtxDone() {
		t.planLoadingStatus(fmt.Sprintf("任务 [%s] 上下文已取消 / Task [%s] Context Cancelled", t.Index, t.Index))
		return utils.Errorf("context is done")
	}

	// Execute the task
	if err := t.execute(); err != nil {
		t.planLoadingStatus(fmt.Sprintf("任务 [%s] 执行出错 / Task [%s] Execution Error", t.Index, t.Index))
		return err
	}

	// Start to wait for user review
	t.planLoadingStatus(fmt.Sprintf("等待用户审查任务 [%s] / Waiting User Review for Task [%s]", t.Index, t.Index))
	ep := t.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	t.EmitInfo("start to wait for user review current task")

	t.EmitRequireReviewForTask(t, ep.GetId())

	log.Infof("task %s waiting for user review event: %v, now status: %v", t.Name, ep.GetId(), t.GetStatus())

	t.DoWaitAgree(t.Ctx, ep)

	// User review finished
	t.planLoadingStatus(fmt.Sprintf("处理任务 [%s] 审查结果 / Processing Review for Task [%s]", t.Index, t.Index))
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
	t.planLoadingStatus(fmt.Sprintf("任务 [%s] 生成总结提示 / Task [%s] Generating Summary Prompt...", t.Index, t.Index))

	summaryPromptWellFormed, err := t.GenerateTaskSummaryPrompt()
	if err != nil {
		t.EmitError("error generating summary prompt: %v", err)
		return fmt.Errorf("error generating summary prompt: %w", err)
	}

	var shortSummary, statusSummary, taskSummary, longSummary string

	// nodeId mapper for different summary keys
	summaryNodeIdMapper := map[string]string{
		"status_summary":     "summary-status",
		"task_short_summary": "summary-short",
		"task_long_summary":  "summary-long",
	}

	// reference material emission tracking (once per transaction, not per key)
	var referenceEmittedOnce sync.Once

	t.planLoadingStatus(fmt.Sprintf("任务 [%s] 等待 AI 生成总结 / Task [%s] Waiting AI Summary...", t.Index, t.Index))
	extractStart := time.Now()
	err = t.CallAiTransaction(summaryPromptWellFormed, t.CallOriginalAI, func(summaryReader *aicommon.AIResponse) error { // 异步过程 使用无 id的 原始ai callback
		action, err := aicommon.ExtractValidActionFromStream(t.Ctx, summaryReader.GetUnboundStreamReader(false), "summary",
			aicommon.WithActionFieldStreamHandler(
				[]string{"status_summary", "task_short_summary", "task_long_summary"},
				func(key string, r io.Reader) {
					// Recover from any panic in stream handler
					defer func() {
						if rec := recover(); rec != nil {
							log.Errorf("summary stream handler for field [%s] panic recovered: %v", key, rec)
						}
					}()

					log.Debugf("summary stream handler started for field [%s]", key)
					t.planLoadingStatus(fmt.Sprintf("任务 [%s] 处理 %s / Task [%s] Processing %s", t.Index, key, t.Index, key))

					// get the corresponding nodeId for this key
					nodeId := summaryNodeIdMapper[key]
					if nodeId == "" {
						nodeId = "summary" // fallback
					}

					// Use TeeReader to capture content while streaming
					var contentBuffer bytes.Buffer
					teeReader := io.TeeReader(utils.UTF8Reader(r), &contentBuffer)

					var event *schema.AiOutputEvent
					var emitErr error
					streamStart := time.Now()
					// Emit stream event with callback for reference material
					event, emitErr = t.EmitDefaultStreamEvent(nodeId, teeReader, t.GetIndex(),
						func() {
							// This callback is called after stream finishes
							log.Debugf("summary stream callback for field [%s] triggered, buffer size: %d, took: %v", key, contentBuffer.Len(), time.Since(streamStart))
							// Emit reference material here (once per transaction)
							if event != nil && summaryPromptWellFormed != "" && contentBuffer.Len() > 0 {
								referenceEmittedOnce.Do(func() {
									streamId := event.GetContentJSONPath(`$.event_writer_id`)
									if streamId != "" {
										_, refErr := t.EmitTextReferenceMaterial(streamId, summaryPromptWellFormed)
										if refErr != nil {
											log.Warnf("emit reference material for summary field [%s] failed: %v", key, refErr)
										}
									}
								})
							}
						},
					)
					if emitErr != nil {
						log.Errorf("failed to emit %s stream event: %v", key, emitErr)
						return
					}
					log.Debugf("summary stream handler for field [%s] emit completed", key)
				},
			))
		log.Infof("ExtractValidActionFromStream for summary completed, took %v", time.Since(extractStart))
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

	t.planLoadingStatus(fmt.Sprintf("任务 [%s] 总结完成 / Task [%s] Summary Completed", t.Index, t.Index))
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

// updateProcessingStatus combines summary (reasoning) and next_movements into StatusSummary
// This ensures both the current status analysis and next action plan are preserved
// to avoid context loss when timeline becomes too long
func (t *AiTask) updateProcessingStatus(summary string, nextMovements string) {
	if summary == "" && nextMovements == "" {
		return
	}

	var statusParts []string

	// Add summary (reasoning) as current status
	if summary != "" {
		statusParts = append(statusParts, fmt.Sprintf("【当前状态】%s", summary))
	}

	// Add next_movements as action plan
	if nextMovements != "" {
		statusParts = append(statusParts, fmt.Sprintf("【下一步计划】%s", nextMovements))
	}

	// Combine both parts into StatusSummary
	t.StatusSummary = strings.Join(statusParts, "\n")

	log.Infof("task %s processing status updated: summary=%q, nextMovements=%q", t.Index, summary, nextMovements)
}
