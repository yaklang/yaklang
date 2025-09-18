package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
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
	skipStatusFallback := utils.NewAtomicBool()
	defer func() {
		r.setCurrentTask(nil) // 处理完成后清除当前任务
		if err := recover(); err != nil {
			log.Errorf("ReAct task processing panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			task.SetStatus(string(TaskStatus_Aborted))
			r.addToTimeline("error", fmt.Sprintf("Task processing panic: %v", err))
		} else {
			if r.config.debugEvent {
				log.Infof("Finished processing task: %s", task.GetId())
			}
			if !skipStatusFallback.IsSet() {
				task.SetStatus(string(TaskStatus_Completed))
			}
		}
	}()

	// 任务状态应该已经在调用前被设置为处理中，这里不需要重复设置

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	r.finished = false
	r.currentIteration = 0
	skipStatus, err := r.executeMainLoop(userInput)
	if err != nil {
		log.Errorf("Task execution failed: %v", err)
		task.SetStatus(string(TaskStatus_Aborted))
		r.addToTimeline("error", fmt.Sprintf("Task execution failed: %v", err))
		return
	}
	if !skipStatus {
		task.SetStatus(string(TaskStatus_Completed))
	}
	skipStatusFallback.SetTo(skipStatus)
}

// generateMainLoopPrompt generates the prompt for the main ReAct loop
func (r *ReAct) generateMainLoopPrompt(
	userQuery string,
	tools []*aitool.Tool,
	disablePlanAndExec bool,
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
		r.config.enablePlanAndExec && !disablePlanAndExec,
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
func (r *ReAct) executeMainLoop(userQuery string) (skipTaskStatusChange bool, err error) {
	currentTask := r.GetCurrentTask()

	skipContextCancel := utils.NewAtomicBool()
	defer func() {
		if skipContextCancel.IsSet() {
			return
		}
		currentTask.Cancel()
	}()

	// show start of iteration in timeline
	iterationTimelineInfo := utils.NewAtomicBool()
	openIterationRecordingOnce := new(sync.Once)
	endIterationRecordingOnce := new(sync.Once)
	endIterationCall := func() {
		if iterationTimelineInfo.IsSet() {
			endIterationRecordingOnce.Do(func() {
				r.addToTimeline("iteration", "======= ReAct loop finished END["+fmt.Sprint(r.currentIteration)+"] =======")
			})
		}
	}
	defer func() {
		endIterationCall()
	}()

	// Reset iteration state for new conversation
	r.currentIteration = 0
	skipTaskStatusChange = false

LOOP:
	for r.currentIteration < r.config.maxIterations {
		if currentTask.IsFinished() {
			break LOOP
		}

		r.currentIteration++
		r.EmitIteration(r.currentIteration, r.config.maxIterations)

		havePlanExecuting := r.GetCurrentPlanExecutionTask() != nil

		// Get available tools
		tools, err := r.config.aiToolManager.GetEnableTools()
		if err != nil {
			log.Errorf("Failed to get available tools: %v", err)
			return false, utils.Errorf("failed to get available tools: %v", err)
		}
		log.Infof("start to generate main loop prompt with %d tools", len(tools))
		prompt := r.generateMainLoopPrompt(userQuery, tools, havePlanExecuting)
		// Use aid.CallAITransaction for robust AI calling with retry and error handling
		var action *aicommon.WaitableAction
		var nextAction aitool.InvokeParams
		var actionErr error
		// Temporarily release lock for AI transaction to prevent deadlocks
		transactionErr := aicommon.CallAITransaction(
			r.config, prompt, r.config.CallAI,
			func(resp *aicommon.AIResponse) error {
				stream := resp.GetOutputStreamReader("re-act-loop", false, r.config.Emitter)
				subCtx, cancel := context.WithCancel(r.config.ctx)
				defer cancel()
				action, actionErr = aicommon.ExtractWaitableActionFromStream(
					subCtx,
					stream,
					ReActActionObject,
					[]string{},
					[]jsonextractor.CallbackOption{
						jsonextractor.WithRegisterFieldStreamHandler(
							"human_readable_thought",
							func(key string, reader io.Reader, parents []string) {
								var output bytes.Buffer
								reader = utils.UTF8Reader(reader)
								reader = io.TeeReader(reader, &output)
								r.config.Emitter.EmitStreamEventEx(
									"re-act-loop",
									time.Now(),
									reader,
									resp.GetTaskIndex(),
									false,
								)
							},
						),
					})
				if actionErr != nil {
					return utils.Errorf("Failed to parse action: %v", actionErr)
				}
				humanRead := action.WaitAnyToString("human_readable_thought")
				if humanRead == "" {
					return utils.Error("human_readable_thought is required but empty in action")
				}

				nextAction = action.WaitObject("next_action")
				actionType := nextAction.GetString("type")
				if actionType == "" {
					return utils.Errorf("Invalid action type: %s", actionType)
				}

				if actionType == string(ActionRequireAIBlueprintForge) {
					blueprintName := nextAction.GetString("blueprint_payload")
					if blueprintName == "" {
						return utils.Error("blueprint_payload is required for ActionRequireAIBlueprintForge but empty")
					}
					if ret, err := r.config.aiBlueprintManager.GetAIForge(blueprintName); ret == nil || err != nil {
						return utils.Errorf("blueprint %s does not exist", blueprintName)
					}
				}

				return nil
			})

		if transactionErr != nil {
			log.Errorf("AI transaction failed (内置错误学习功能): %v", transactionErr)
			continue
		}
		thought := action.WaitString("human_readable_thought")

		saveIterationInfoIntoTimeline := func() {
			// allow iteration info to be added to timeline
			r.addToTimeline("iteration", fmt.Sprintf(
				"======== ReAct iteration %d ========\n"+
					"%v", r.currentIteration, thought,
			))
			openIterationRecordingOnce.Do(func() {
				iterationTimelineInfo.SetTo(true)
			})
		}

		// Emit human readable thought
		r.EmitThought(thought)
		r.PushCumulativeSummaryHandle(func() string {
			return action.WaitString("cumulative_summary")
		})
		actionType := ActionType(nextAction.GetString("type"))
		switch actionType {
		case ActionDirectlyAnswer:
			answerPayload := nextAction.GetString("answer_payload")
			r.EmitResult(answerPayload)
			currentTask.SetResult(strings.TrimSpace(answerPayload))
			r.addToTimeline("directly_answer", fmt.Sprintf("user input: \n"+
				"%s\n"+
				"ai directly answer:\n"+
				"%v",
				utils.PrefixLines(currentTask.GetUserInput(), "  > "),
				utils.PrefixLines(answerPayload, "  | "),
			))
			endIterationCall()
			currentTask.SetStatus(string(TaskStatus_Completed))
			continue
		case ActionRequireTool:
			saveIterationInfoIntoTimeline()

			toolPayload := nextAction.GetString("tool_require_payload")
			log.Infof("Requesting tool: %s", toolPayload)
			toolcallResult, directlyAnswerRequired, err := r.handleRequireTool(toolPayload)
			if err != nil {
				r.addToTimeline("error-calling-tool", fmt.Sprintf("Failed to handle require tool[%v]: %v", toolPayload, err))
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
				r.addToTimeline(
					"directly_answer_required",
					"ai call-tool step is aborted due user requirement",
				)
				result, err := r.requireDirectlyAnswer(
					userQuery+
						"\n===========\n"+
						"**用户要求 AI 直接回答，所以在本次回答中，不允许使用工具和其他复杂方法手段回答**", tools)
				if err != nil {
					endIterationCall()
					currentTask.SetStatus(string(TaskStatus_Aborted))
					log.Errorf("Failed to require directly answer: %v", err)
					continue
				}
				if result == "" {
					endIterationCall()
					currentTask.SetStatus(string(TaskStatus_Aborted))
					log.Errorf("Failed to require directly answer: %v", err)
					continue
				}
				currentTask.SetResult(strings.TrimSpace(result) + " (force directly answer)")
				endIterationCall()
				currentTask.SetStatus(string(TaskStatus_Completed))
				continue
			}

			if nextAction.GetBool("middle_step") {
				r.EmitInfo("middle step, tool")
				continue
			}

			// Tool executed successfully, now verify if user needs are satisfied
			// Temporarily release the lock before calling verification to avoid deadlock
			satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, true, toolPayload)
			if err != nil {
				endIterationCall()
				currentTask.SetStatus(string(TaskStatus_Aborted))
				continue
			} else if satisfied {
				r.EmitResult(finalResult)
				endIterationCall()
				currentTask.SetStatus(string(TaskStatus_Completed))
				continue
			} else {
				// User needs not satisfied, continue loop
				log.Infof("User needs not fully satisfied, continuing analysis...")
				r.addToTimeline(
					"reason",
					fmt.Sprintf("User needs not fully satisfied after tool call, continuing analysis...\n"+
						"%v", finalResult,
					))
			}
		case ActionRequireAIBlueprintForge:
			saveIterationInfoIntoTimeline()
			forgeName := nextAction.GetString("blueprint_payload")
			r.addToTimeline("plan", fmt.Sprintf("ai-forge-name(blueprint): %v is requested", forgeName))

			if havePlanExecuting {
				r.Emitter.EmitWarning("existed plan execution task is running, cannot start a new one")
				r.addToTimeline("plan_warning", "a plan execution task is already running, cannot start a new one")
				return false, utils.Errorf("a plan execution task is already running, cannot start a new one (even through ai-blueprint is requested)")
			}

			ins, forgeParams, err := r.invokeBlueprint(forgeName)
			if err != nil {
				r.finished = true
				r.addToTimeline("plan_error", fmt.Sprintf("failed to invoke ai-blueprint[%v]: %v", forgeName, err))
				return false, utils.Errorf("failed to invoke ai-blueprint[%v]: %v", forgeName, err)
			}

			r.addToTimeline("ai-blueprint", fmt.Sprintf(
				`ai-blueprint: %v is invoked with params: %v`,
				forgeName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256),
			))

			r.SetCurrentPlanExecutionTask(currentTask)
			skipContextCancel.SetTo(true) // Plan execution will manage the context
			taskStarted := make(chan struct{})
			timelineStartPlanChan := make(chan struct{})
			go func() {
				defer func() {
					select {
					case <-timelineStartPlanChan:
						r.addToTimeline("plan_execution", fmt.Sprintf("ai-blueprint: %v is finished", utils.ShrinkString(forgeName, 128)))
					}
					currentTask.Cancel() // Ensure the task context is cancelled after plan execution.
					currentTask.SetStatus(string(TaskStatus_Completed))
					r.SetCurrentPlanExecutionTask(nil)
				}()
				if err := r.invokePlanAndExecute(taskStarted, currentTask.GetContext(), "", ins.ForgeName, forgeParams); err != nil {
					log.Errorf("Plan execution failed: %v", err)
				}
			}()
			select {
			case <-taskStarted:
				r.addToTimeline("plan_execution", fmt.Sprintf("ai-blueprint: %v is started", forgeName))
				close(timelineStartPlanChan)
				log.Infof("plan execution task started")
				skipTaskStatusChange = true
				break LOOP
			}
		case ActionRequestPlanExecution:
			saveIterationInfoIntoTimeline()

			planPayload := nextAction.GetString("plan_request_payload")
			if havePlanExecuting {
				r.Emitter.EmitWarning("existed plan execution task is running, cannot start a new one")
				r.addToTimeline("plan_warning", "a plan execution task is already running, cannot start a new one")
				return false, utils.Errorf("a plan execution task is already running, cannot start a new one")
			}
			r.SetCurrentPlanExecutionTask(currentTask)
			log.Infof("Requesting plan execution: %s, start to create p-e coordinator", planPayload)
			skipContextCancel.SetTo(true) // Plan execution will manage the context
			taskStarted := make(chan struct{})
			timelineStartPlanChan := make(chan struct{})
			go func() {
				defer func() {
					select {
					case <-timelineStartPlanChan:
						r.addToTimeline("plan_execution", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
					}
					currentTask.Cancel() // Ensure the task context is cancelled after plan execution.
					currentTask.SetStatus(string(TaskStatus_Completed))
					r.SetCurrentPlanExecutionTask(nil)
				}()
				if err := r.invokePlanAndExecute(taskStarted, currentTask.GetContext(), planPayload, "", nil); err != nil {
					log.Errorf("Plan execution failed: %v", err)
				}
			}()
			select {
			case <-taskStarted:
				r.addToTimeline("plan_execution", fmt.Sprintf("plan: %v is started", planPayload))
				close(timelineStartPlanChan)
				log.Infof("plan execution task started")
				skipTaskStatusChange = true
				break LOOP
			}
		case ActionAskForClarification:
			saveIterationInfoIntoTimeline()
			suggestion := r.invokeAskForClarification(nextAction)
			if suggestion == "" {
				suggestion = "user did not provide a valid suggestion, using default 'continue' action"
			}
			satisfied, finalResult, err := r.verifyUserSatisfaction(userQuery, false, suggestion)
			if err != nil {
				endIterationCall()
				currentTask.SetStatus(string(TaskStatus_Aborted))
				continue
			} else if satisfied {
				r.EmitResult(finalResult)
				endIterationCall()
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
	return skipTaskStatusChange, nil
}
