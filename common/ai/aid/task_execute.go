package aid

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
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

	// Record task start time for duration calculation
	t.taskStartTime = time.Now()

	// Record timeline baseline before task execution starts
	// We use the global timeline differ to track changes during task execution
	if t.Coordinator != nil {
		timeline := t.Coordinator.ContextProvider.GetTimelineInstance()
		if timeline != nil {
			// Create a new TimelineDiffer for this task to track changes during execution
			// This differ will be used to calculate the diff at the end of task execution
			taskTimelineDiffer := aicommon.NewTimelineDiffer(timeline)
			taskTimelineDiffer.SetBaseline()
			// Store the differ for later use in generateTaskSummary
			t.taskTimelineDiffer = taskTimelineDiffer
			log.Debugf("task %s timeline baseline recorded, baseline content length: %d", t.Index, len(taskTimelineDiffer.GetLastDump()))
		}
	}

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

				err := t.generateTaskSummary(summary, nextMovements)
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

			if t.Coordinator != nil && summary != "" {
				timelineMsg := fmt.Sprintf(
					"[task-verification] Task %s iteration %d verification: not yet complete. Reason: %s",
					t.Index, iteration, summary,
				)
				if nextMovements != "" {
					timelineMsg += fmt.Sprintf(" | Suggested next steps: %s", nextMovements)
				}
				t.Coordinator.Timeline.PushText(
					t.Coordinator.AcquireId(),
					timelineMsg,
				)
			}
		}
		}),
		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedback *bytes.Buffer, nonce string) (string, error) {
			currentIteration := loop.GetCurrentIterationIndex()

			var lastVerificationInfo string
			if lastRecord := loop.GetLastSatisfactionRecordFull(); lastRecord != nil {
				lastVerificationInfo = fmt.Sprintf("satisfied=%v, reasoning=%s", lastRecord.Satisfactory, lastRecord.Reason)
				if lastRecord.NextMovements != "" {
					lastVerificationInfo += fmt.Sprintf(", next_movements=%s", lastRecord.NextMovements)
				}
			}

			var recentActionsSummary string
			if recentActions := loop.GetLastNAction(3); len(recentActions) > 0 {
				var parts []string
				for _, a := range recentActions {
					parts = append(parts, fmt.Sprintf("iter%d: %s(%s)", a.IterationIndex, a.ActionType, a.ActionName))
				}
				recentActionsSummary = strings.Join(parts, " -> ")
			}

			reactiveData := utils.MustRenderTemplate(`
当前 Plan-Execution 模式进度信息：

<|PROGRESS_TASK_{{.Nonce}}|>
{{ .Progress }}

--- CURRENT_TASK ---
{{ .CurrentProgress }}
--- CURRENT_TASK_END ---

<|PROGRESS_TASK_END_{{ .Nonce }}|>

--- TASK_ITERATION_INFO ---
当前子任务迭代次数: {{ .CurrentIteration }}
{{ if .StatusSummary }}当前状态分析: {{ .StatusSummary }}{{ end }}
{{ if .LastVerificationInfo }}上次验证结果: {{ .LastVerificationInfo }}{{ end }}
{{ if .FeedbackMessages }}
最近反馈信息:
{{ .FeedbackMessages }}
{{ end }}
{{ if .RecentActions }}最近执行动作: {{ .RecentActions }}{{ end }}
{{ if gt .CurrentIteration 5 }}
** 警告: 当前子任务已执行 {{ .CurrentIteration }} 次迭代，请认真评估：
  1. 任务目标是否实际上已经完成？如果工具已返回足够结果，请允许任务完成。
  2. 当前策略是否有效？如果反复失败，请更换工具或方法。
  3. 不要重复执行相同的操作，这会浪费迭代次数。**
{{ end }}
--- TASK_ITERATION_INFO_END ---

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
				"Progress":             t.rootTask.Progress(),
				"CurrentProgress":      t.Progress(),
				"Nonce":                nonce,
				"CurrentIteration":     currentIteration,
				"FeedbackMessages":     feedback.String(),
				"StatusSummary":        t.StatusSummary,
				"LastVerificationInfo": lastVerificationInfo,
				"RecentActions":        recentActionsSummary,
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

	if t.GetStatus() != aicommon.AITaskState_Skipped {
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
	} else {
		t.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", t.Index, t.Index))
		t.EmitInfo("task %s was skipped by user, skip review", t.Name)
		log.Infof("task %s was skipped by user, skip review", t.Name)
	}

	return nil
}

func (t *AiTask) generateTaskSummary(summary, nextMovements string) error {
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
					nodeId, ok := summaryNodeIdMapper[key]
					if nodeId == "" || !ok {
						nodeId = "summary" // fallback
					}

					// Use TeeReader to capture content while streaming
					var contentBuffer bytes.Buffer
					teeReader := io.TeeReader(utils.JSONStringReader(utils.UTF8Reader(r)), &contentBuffer)

					var event *schema.AiOutputEvent
					var emitErr error
					streamStart := time.Now()

					onEnd := func() {
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
					}

					// Emit stream event with callback for reference material
					if nodeId == "summary-long" {
						event, emitErr = t.EmitTextMarkdownStreamEvent(nodeId, teeReader, t.GetIndex(), onEnd)
					} else {
						event, emitErr = t.EmitDefaultStreamEvent(nodeId, teeReader, t.GetIndex(), onEnd)
					}

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

	// Save timeline diff and result summary artifacts
	if err := t.saveTaskArtifacts(summary, nextMovements, statusSummary, taskSummary, shortSummary, longSummary); err != nil {
		log.Warnf("failed to save task artifacts for task %s: %v", t.Index, err)
		// Don't return error, as summary generation is already successful
	}

	return nil
}

// saveTaskArtifacts saves timeline diff and result summary to files in the task directory
func (t *AiTask) saveTaskArtifacts(summary, nextMovements, statusSummary, taskSummary, shortSummary, longSummary string) error {
	// Get workdir
	workdir := ""
	if t.Coordinator != nil && t.Coordinator.Workdir != "" {
		workdir = t.Coordinator.Workdir
	}
	if workdir == "" && t.Coordinator != nil {
		workdir = t.Coordinator.GetOrCreateWorkDir()
	}
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}

	// Build task directory path: task_{index}_{name}
	taskIndex := t.Index
	if taskIndex == "" {
		taskIndex = "0"
	}
	taskDir := filepath.Join(workdir, aicommon.BuildTaskDirName(taskIndex, t.GetSemanticIdentifier()))

	// Ensure task directory exists
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return fmt.Errorf("failed to create task directory %s: %w", taskDir, err)
	}

	// Save timeline diff
	if err := t.saveTimelineDiff(taskDir); err != nil {
		log.Warnf("failed to save timeline diff for task %s: %v", t.Index, err)
	}

	// Save result summary
	if err := t.saveResultSummary(taskDir, summary, nextMovements, statusSummary, taskSummary, shortSummary, longSummary); err != nil {
		log.Warnf("failed to save result summary for task %s: %v", t.Index, err)
	}

	return nil
}

// saveTimelineDiff saves the timeline diff to task_{{index}}_timeline_diff.txt
// It gets the diff from the ReactLoop which tracks timeline changes during task execution
func (t *AiTask) saveTimelineDiff(taskDir string) error {
	// Get task index for filename
	taskIndex := t.Index
	if taskIndex == "" {
		taskIndex = "0"
	}
	// Sanitize task index for filename (replace - with _)
	safeTaskIndex := strings.ReplaceAll(taskIndex, "-", "_")

	var diff string
	var err error

	// Try to get diff from ReactLoop first (this is the correct source)
	if t.GetReActLoop() != nil {
		diff, err = t.GetReActLoop().GetTimelineDiff()
		if err != nil {
			log.Warnf("failed to get timeline diff from ReactLoop: %v", err)
		}
	}

	// Fallback to taskTimelineDiffer if ReactLoop didn't provide a diff
	if diff == "" && t.taskTimelineDiffer != nil {
		diff, err = t.taskTimelineDiffer.Diff()
		if err != nil {
			log.Warnf("failed to calculate timeline diff from taskTimelineDiffer: %v", err)
		}
	}

	// Build content with header information
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("# Task %s Timeline Diff\n", taskIndex))
	contentBuilder.WriteString(fmt.Sprintf("# Generated at: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	contentBuilder.WriteString("\n")

	if diff == "" {
		// If diff is empty, provide debug information
		contentBuilder.WriteString("## Note: No changes detected during task execution\n\n")

		// Try to get current timeline content for debugging
		if t.taskTimelineDiffer != nil {
			currentDump := t.taskTimelineDiffer.GetCurrentDump()
			lastDump := t.taskTimelineDiffer.GetLastDump()
			contentBuilder.WriteString(fmt.Sprintf("Baseline content length: %d bytes\n", len(lastDump)))
			contentBuilder.WriteString(fmt.Sprintf("Current content length: %d bytes\n", len(currentDump)))

			if currentDump != "" {
				contentBuilder.WriteString("\n## Current Timeline Content:\n")
				contentBuilder.WriteString(currentDump)
			} else {
				contentBuilder.WriteString("\n(Timeline is empty)\n")
			}
		}
	} else {
		contentBuilder.WriteString("## Timeline Changes:\n\n")
		contentBuilder.WriteString(diff)
	}

	// Save to file with task index in filename
	timelineDiffPath := filepath.Join(taskDir, fmt.Sprintf("task_%s_timeline_diff.txt", safeTaskIndex))
	if err := os.WriteFile(timelineDiffPath, []byte(contentBuilder.String()), 0644); err != nil {
		return fmt.Errorf("failed to write timeline diff file: %w", err)
	}

	// Emit pin filename event
	if t.GetEmitter() != nil {
		t.GetEmitter().EmitPinFilename(timelineDiffPath)
		log.Infof("saved timeline diff to file: %s (diff length: %d)", timelineDiffPath, len(diff))
	}

	return nil
}

// saveResultSummary saves the result summary to task_{{index}}_result_summary.txt
func (t *AiTask) saveResultSummary(taskDir string, summary, nextMovements, statusSummary, taskSummary, shortSummary, longSummary string) error {
	// Get task index for filename
	taskIndex := t.Index
	if taskIndex == "" {
		taskIndex = "0"
	}
	// Sanitize task index for filename (replace - with _)
	safeTaskIndex := strings.ReplaceAll(taskIndex, "-", "_")

	var contentBuilder strings.Builder

	// === Header Section ===
	contentBuilder.WriteString("=" + strings.Repeat("=", 59) + "\n")
	contentBuilder.WriteString(fmt.Sprintf(" Task %s Result Summary\n", taskIndex))
	contentBuilder.WriteString("=" + strings.Repeat("=", 59) + "\n\n")

	// === Basic Information Section ===
	contentBuilder.WriteString("## Basic Information\n\n")
	contentBuilder.WriteString(fmt.Sprintf("Task Index: %s\n", taskIndex))
	contentBuilder.WriteString(fmt.Sprintf("Task Name: %s\n", t.Name))
	contentBuilder.WriteString(fmt.Sprintf("Task Goal: %s\n", t.Goal))
	contentBuilder.WriteString(fmt.Sprintf("Generated At: %s\n", time.Now().Format("2006-01-02 15:04:05")))

	// Calculate and display execution duration
	if !t.taskStartTime.IsZero() {
		duration := time.Since(t.taskStartTime)
		contentBuilder.WriteString(fmt.Sprintf("Execution Duration: %s\n", formatDuration(duration)))
		contentBuilder.WriteString(fmt.Sprintf("Start Time: %s\n", t.taskStartTime.Format("2006-01-02 15:04:05")))
		contentBuilder.WriteString(fmt.Sprintf("End Time: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	}

	// Task status
	contentBuilder.WriteString(fmt.Sprintf("Task Status: %s\n", t.GetStatus()))

	// Tool call statistics
	toolCallResults := t.GetAllToolCallResults()
	successCount := 0
	failCount := 0
	for _, result := range toolCallResults {
		if result.Success {
			successCount++
		} else {
			failCount++
		}
	}
	contentBuilder.WriteString(fmt.Sprintf("Total Tool Calls: %d (Success: %d, Failed: %d)\n", len(toolCallResults), successCount, failCount))

	contentBuilder.WriteString("\n")

	// === Task Input Section ===
	contentBuilder.WriteString("## Task Input\n\n")
	userInput := t.GetUserInput()
	if userInput != "" {
		// Limit input display to avoid very long content
		if len(userInput) > 2000 {
			contentBuilder.WriteString(userInput[:2000])
			contentBuilder.WriteString("\n... (truncated, total " + fmt.Sprintf("%d", len(userInput)) + " chars)\n")
		} else {
			contentBuilder.WriteString(userInput)
		}
	} else {
		contentBuilder.WriteString("(No input provided)\n")
	}
	contentBuilder.WriteString("\n")

	// === Progress Information Section ===
	contentBuilder.WriteString("## Progress Information\n\n")
	if t.rootTask != nil {
		progress := t.rootTask.Progress()
		if progress != "" {
			contentBuilder.WriteString(progress)
		} else {
			contentBuilder.WriteString("(No progress information available)\n")
		}
	} else {
		contentBuilder.WriteString("(No root task available)\n")
	}
	contentBuilder.WriteString("\n")

	// === Summary Results Section ===
	contentBuilder.WriteString("## Summary Results\n\n")

	hasContent := false
	if summary != "" {
		contentBuilder.WriteString("### Summary\n")
		contentBuilder.WriteString(summary)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}
	if nextMovements != "" {
		contentBuilder.WriteString("### Next Movements\n")
		contentBuilder.WriteString(nextMovements)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}
	if statusSummary != "" {
		contentBuilder.WriteString("### Status Summary\n")
		contentBuilder.WriteString(statusSummary)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}
	if taskSummary != "" {
		contentBuilder.WriteString("### Task Summary\n")
		contentBuilder.WriteString(taskSummary)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}
	if shortSummary != "" {
		contentBuilder.WriteString("### Short Summary\n")
		contentBuilder.WriteString(shortSummary)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}
	if longSummary != "" {
		contentBuilder.WriteString("### Long Summary\n")
		contentBuilder.WriteString(longSummary)
		contentBuilder.WriteString("\n\n")
		hasContent = true
	}

	if !hasContent {
		contentBuilder.WriteString("(No summary content available)\n\n")
	}

	// === Footer ===
	contentBuilder.WriteString("=" + strings.Repeat("=", 59) + "\n")
	contentBuilder.WriteString(" End of Task " + taskIndex + " Result Summary\n")
	contentBuilder.WriteString("=" + strings.Repeat("=", 59) + "\n")

	// Save to file with task index in filename
	resultSummaryPath := filepath.Join(taskDir, fmt.Sprintf("task_%s_result_summary.txt", safeTaskIndex))
	if err := os.WriteFile(resultSummaryPath, []byte(contentBuilder.String()), 0644); err != nil {
		return fmt.Errorf("failed to write result summary file: %w", err)
	}

	// Emit pin filename event
	if t.GetEmitter() != nil {
		t.GetEmitter().EmitPinFilename(resultSummaryPath)
		log.Infof("saved result summary to file: %s", resultSummaryPath)
	}

	return nil
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.2f seconds", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%d min %d sec", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%d hr %d min %d sec", hours, minutes, seconds)
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
