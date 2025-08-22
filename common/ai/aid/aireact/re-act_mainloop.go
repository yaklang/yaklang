package aireact

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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
	var enableUserInteractive bool = true
	if r.currentUserInteractiveCount >= r.config.userInteractiveLimitedTimes {
		enableUserInteractive = false
	}

	// Use the prompt manager to generate the prompt
	prompt, err := r.promptManager.GenerateLoopPrompt(
		userQuery,
		enableUserInteractive,
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
				// handle directly answer requred
				// force
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
			log.Infof("Requesting plan execution: %s, start to create p-e coordinator", planPayload)
			if err := r.invokePlanAndExecute(planPayload); err != nil {
				log.Errorf("Plan execution failed: %v", err)
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
