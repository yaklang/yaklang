package aireact

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// processReActFromQueue 处理队列中的下一个任务
func (r *ReAct) processReActFromQueue() {
	if r.taskQueue.IsEmpty() {
		return
	}

	// 如果正在处理任务，直接返回
	if r.IsProcessingReAct() {
		return
	}

	// 从队列获取下一个任务
	log.Infof("start to get first task from queue for ReAct instance: %s", r.config.id)
	nextTask := r.taskQueue.GetFirst()
	if nextTask == nil {
		return
	}
	r.setCurrentTask(nextTask)
	// 更新任务状态为处理中
	nextTask.SetStatus(string(TaskStatus_Processing))
	if r.config.debugEvent {
		log.Infof("Processing task from queue: %s", nextTask.GetId())
	}
	// 异步处理任务
	r.processReActTask(nextTask)
}

// processReActTask 处理单个 Task
func (r *ReAct) processReActTask(task *Task) {
	defer func() {
		r.setCurrentTask(nil) // 处理完成后清除当前任务
		if err := recover(); err != nil {
			log.Errorf("ReAct task processing panic: %v", err)
			task.SetStatus(string(TaskStatus_Aborted))
			r.addToTimeline("error", fmt.Sprintf("Task processing panic: %v", err), task.GetId())
		}
	}()

	// 任务状态应该已经在调用前被设置为处理中，这里不需要重复设置

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	r.finished = false
	r.currentIteration = 0
	err := r.executeMainLoop(userInput)
	if err != nil {
		log.Errorf("Task execution failed: %v", err)
		task.SetStatus(string(TaskStatus_Aborted))
		r.addToTimeline("error", fmt.Sprintf("Task execution failed: %v", err), task.GetId())
	} else {
		task.SetStatus(string(TaskStatus_Completed))
		r.addToTimeline("completed", fmt.Sprintf("Task completed: %s", task.GetUserInput()), task.GetId())
	}
}

// generateMainLoopPrompt generates the prompt for the main ReAct loop
func (r *ReAct) generateMainLoopPrompt(
	userQuery string,
	tools []*aitool.Tool,
) string {
	// Generate prompt for main loop
	var enableUserInteractive = r.config.enableUserInteract
	if enableUserInteractive && (r.currentUserInteractiveCount >= r.config.userInteractiveLimitedTimes) {
		enableUserInteractive = false
	}

	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateLoopPrompt(
		userQuery,
		enableUserInteractive,
		r.config.enablePlanAndExec,
		r.currentUserInteractiveCount,
		r.config.userInteractiveLimitedTimes,
		tools,
	)
	if err != nil {
		// Fallback to basic prompt if template fails
		log.Errorf("Failed to generate loop prompt from template: %v", err)
		return fmt.Sprintf("User Query: %s\nPlease respond with a JSON object for ReAct action.", userQuery)
	}
	return prompt
}

// executeMainLoop executes the main ReAct loop
func (r *ReAct) executeMainLoop(userQuery string) error {
	currentTask := r.GetCurrentTask()

	skipContextCancel := utils.NewAtomicBool()
	defer func() {
		if skipContextCancel.IsSet() {
			return
		}
		currentTask.Cancel()
	}()

	// Reset iteration state for new conversation
	r.currentIteration = 0
	for r.currentIteration < r.config.maxIterations {
		if currentTask.IsFinished() {
			break
		}
		r.currentIteration++
		r.EmitIteration(r.currentIteration, r.config.maxIterations)

		// Get available tools
		tools, err := r.config.aiToolManager.GetEnableTools()
		if err != nil {
			log.Errorf("Failed to get available tools: %v", err)
			return utils.Errorf("failed to get available tools: %v", err)
		}
		prompt := r.generateMainLoopPrompt(userQuery, tools)
		// Use aid.CallAITransaction for robust AI calling with retry and error handling
		var action *aicommon.Action
		var actionErr error
		// Temporarily release lock for AI transaction to prevent deadlocks
		transactionErr := aicommon.CallAITransaction(
			r.config, prompt, r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader("re-act-loop", false, r.config.Emitter)
				action, actionErr = aicommon.ExtractActionFromStream(stream, ReActActionObject)
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}
				humanRead := action.GetAnyToString("human_readable_thought")
				if humanRead == "" {
					return utils.Error("human_readable_thought is required but empty in action")
				}
				actionType := action.GetInvokeParams("next_action").GetString("type")
				if actionType == "" {
					return utils.Errorf("Invalid action type: %s", action.GetInvokeParams("type"))
				}
				return nil
			})

		if transactionErr != nil {
			log.Errorf("AI transaction failed (内置错误学习功能): %v", transactionErr)
			continue
		}
		// Emit human readable thought
		r.EmitThought(action.GetString("human_readable_thought"))
		newSummary := action.GetString("cumulative_summary")
		if newSummary != "" {
			r.cumulativeSummary = newSummary
		}
		nextAction := action.GetInvokeParams("next_action")
		actionType := ActionType(nextAction.GetString("type"))
		switch actionType {
		case ActionDirectlyAnswer:
			answerPayload := nextAction.GetString("answer_payload")
			r.EmitResult(answerPayload)
			currentTask.SetStatus(string(TaskStatus_Completed))
			continue
		case ActionRequireTool:
			toolPayload := nextAction.GetString("tool_require_payload")
			log.Infof("Requesting tool: %s", toolPayload)
			toolcallResult, directlyAnswerRequired, err := r.handleRequireTool(toolPayload)
			if err != nil {
				currentTask.SetStatus(string(TaskStatus_Processing))
				log.Errorf("Failed to handle require tool: %v, retry it", err)
				continue
			}

			var payload bytes.Buffer
			if !directlyAnswerRequired {
				payload.WriteString(toolcallResult.StringWithoutID())
			} else {
				// handle directly answer required
				// forcely set satisfied to true
			}

			// Tool executed successfully, now verify if user needs are satisfied
			// Temporarily release the lock before calling verification to avoid deadlock
			satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, true, toolPayload)
			if err != nil {
				currentTask.SetStatus(string(TaskStatus_Aborted))
				continue
			} else if satisfied {
				r.EmitResult(finalResult)
				currentTask.SetStatus(string(TaskStatus_Completed))
				continue
			} else {
				// User needs not satisfied, continue loop
				log.Infof("User needs not fully satisfied, continuing analysis...")
			}
		case ActionRequestPlanExecution:
			planPayload := action.GetInvokeParams("next_action").GetString("plan_request_payload")
			if r.GetCurrentPlanExecutionTask() != nil {
				// ask user to determine kill or wait
				ep := r.config.epm.CreateEndpointWithEventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE)
				ep.SetDefaultSuggestionContinue()
				result := map[string]any{
					"id":       ep.GetId(),
					"question": "An existed plan execution task is running, do you want to terminate it and start a new one?",
					"options": []map[string]any{
						{"index": 1, "prompt_title": "中断正在执行的规划任务，开始新的规划任务"},
						{"index": 2, "prompt_title": "保留正在执行的规划任务，放弃本次规划请求"},
						{"index": 3, "prompt_title": "等待任务执行完再执行这个任务"},
					},
				}
				ep.SetReviewMaterials(result)
				r.config.EmitInteractiveJSON(
					ep.GetId(),
					schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE,
					"require-user-interact",
					result,
				)
				ctx := r.config.GetContext()
				ctx = utils.SetContextKey(ctx, SKIP_AI_REVIEW, true)
				r.config.DoWaitAgree(ctx, ep)
				params := ep.GetParams()
				r.config.EmitInteractiveRelease(ep.GetId(), params)
				r.config.CallAfterInteractiveEventReleased(ep.GetId(), params)
				suggestion := params.GetAnyToString("suggestion")
				_ = suggestion
			}
			r.SetCurrentPlanExecutionTask(currentTask)
			log.Infof("Requesting plan execution: %s, start to create p-e coordinator", planPayload)
			skipContextCancel.SetTo(true) // Plan execution will manage the context
			taskStarted := make(chan struct{})
			go func() {
				defer func() {
					currentTask.Cancel() // Ensure the task context is cancelled after plan execution.
					r.SetCurrentPlanExecutionTask(nil)
				}()
				if err := r.invokePlanAndExecute(taskStarted, currentTask.GetContext(), planPayload); err != nil {
					log.Errorf("Plan execution failed: %v", err)
				}
			}()
			select {
			case <-taskStarted:
			}
		case ActionAskForClarification:
			suggestion := r.invokeAskForClarification(action)
			if suggestion == "" {
				suggestion = "user did not provide a valid suggestion, using default 'continue' action"
			}
			satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, false, suggestion)
			if err != nil {
				currentTask.SetStatus(string(TaskStatus_Aborted))
				continue
			} else if satisfied {
				r.EmitResult(finalResult)
				currentTask.SetStatus(string(TaskStatus_Completed))
				continue
			} else {
				// User needs not satisfied, continue loop
				log.Infof("User needs not fully satisfied, continuing analysis...")
				continue
			}
		default:
			r.EmitError("unknown action type: %v", actionType)
			r.finished = true
		}
	}
	if r.currentIteration >= r.config.maxIterations {
		r.EmitWarning("Too many iterations[%v] is reached, stopping ReAct loop, max: %v", r.currentIteration, r.config.maxIterations)
	}
	return nil
}
