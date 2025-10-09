package aireact

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/schema"
	"io"
	"time"

	"github.com/google/uuid"

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
	nextTask.SetStatus(aicommon.AITaskState_Processing)
	if r.config.debugEvent {
		log.Infof("Processing task from queue: %s", nextTask.GetId())
	}
	// 异步处理任务
	r.processReActTask(nextTask)
}

// processReActTask 处理单个 Task
func (r *ReAct) processReActTask(task aicommon.AIStatefulTask) {
	skipStatusFallback := utils.NewAtomicBool()
	defer func() {
		r.SaveTimeline()
		r.setCurrentTask(nil) // 处理完成后清除当前任务
		if err := recover(); err != nil {
			log.Errorf("ReAct task processing panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
			task.SetStatus(aicommon.AITaskState_Aborted)
			r.AddToTimeline("error", fmt.Sprintf("Task processing panic: %v", err))
		} else {
			if r.config.debugEvent {
				log.Infof("Finished processing task: %s", task.GetId())
			}
			if !skipStatusFallback.IsSet() {
				task.SetStatus(aicommon.AITaskState_Completed)
			}
		}
	}()

	// 任务状态应该已经在调用前被设置为处理中，这里不需要重复设置

	// 从任务中提取用户输入
	userInput := task.GetUserInput()

	r.currentIteration = 0
	skipStatus, err := r.executeMainLoop(userInput)
	if err != nil {
		log.Errorf("Task execution failed: %v", err)
		task.SetStatus(aicommon.AITaskState_Aborted)
		r.AddToTimeline("error", fmt.Sprintf("Task execution failed: %v", err))
		return
	}
	if !skipStatus {
		task.SetStatus(aicommon.AITaskState_Completed)
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
		r.config.enhanceKnowledgeManager != nil && !r.config.disableEnhanceDirectlyAnswer,
		true,
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

func (r *ReAct) executeMainLoop(userQuery string) (bool, error) {
	mainloop, err := reactloops.CreateLoopByName(
		loop_default.LOOP_NAME_DEFAULT,
		r,
	)

	if err != nil {
		return false, utils.Errorf("failed to create main loop runtime instance: %v", err)
	}
	currentTask := r.GetCurrentTask()
	currentTask.SetUserInput(userQuery)
	if r.GetCurrentPlanExecutionTask() != nil {
		// have async plan execution task running, disable plan and exec in main loop
		mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
		mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
	}
	err = mainloop.ExecuteWithExistedTask(currentTask)
	if err != nil {
		return false, err
	}
	if currentTask.IsAsyncMode() {
		r.SetCurrentPlanExecutionTask(currentTask)
		mainloop.OnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
			r.SetCurrentPlanExecutionTask(nil)
		})
		mainloop.GetConfig()
	}
	return currentTask.IsAsyncMode(), nil
}

func (r *ReAct) EnhanceKnowledgeAnswer(ctx context.Context, userQuery string) (string, error) {
	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	if config.enhanceKnowledgeManager == nil {
		return "", utils.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check main loop prompt!")
	}

	enhanceData, err := config.enhanceKnowledgeManager.FetchKnowledge(r.config.ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		config.enhanceKnowledgeManager.AppendKnowledge(currentTask.GetId(), enhanceDatum)
	}

	queryPrompt, err := r.promptManager.GenerateDirectlyAnswerPrompt(userQuery, nil, r.DumpCurrentEnhanceData())
	if err != nil {
		return "", err
	}

	var finalResult string
	err = aicommon.CallAITransaction(
		r.config,
		queryPrompt,
		r.config.CallAI,
		func(rsp *aicommon.AIResponse) error {
			stream := rsp.GetOutputStreamReader("directly_answer", true, r.Emitter)
			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			waitAction, err := aicommon.ExtractWaitableActionFromStream(
				subCtx,
				stream, "object", []string{},
				[]jsonextractor.CallbackOption{
					jsonextractor.WithRegisterFieldStreamHandler(
						"answer_payload",
						func(key string, reader io.Reader, parents []string) {
							var output bytes.Buffer
							reader = utils.JSONStringReader(utils.UTF8Reader(reader))
							reader = io.TeeReader(reader, &output)
							r.config.Emitter.EmitStreamEventEx(
								"re-act-loop-answer-payload",
								time.Now(),
								reader,
								rsp.GetTaskIndex(),
								false,
								func() {
									r.EmitResultAfterStream(output.String())
								},
							)
						},
					),
				})
			if err != nil {
				return err
			}
			nextAction := waitAction.WaitObject("next_action") // ensure next_action is fully received
			if nextAction == nil || nextAction.GetString("answer_payload") == "" {
				return utils.Error("answer_payload is required but empty in action")
			}
			finalResult = nextAction.GetString("answer_payload")
			return nil
		},
	)
	r.EmitTextArtifact("enhance_directly_answer", finalResult)
	return finalResult, err
}
