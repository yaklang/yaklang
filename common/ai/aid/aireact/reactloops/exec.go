package reactloops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReActLoop) buildActionTagOption(streamWG *sync.WaitGroup, taskIndex string, nonce string) []aicommon.ActionMakerOption {
	var emitter = r.GetEmitter()
	tagFields := r.aiTagFields.Copy()
	for _, i := range r.GetAllActions() {
		for _, field := range i.AITagStreamFields {
			tagFields.Set(field.TagName, field)
		}
	}
	var actionOptions []aicommon.ActionMakerOption
	actionOptions = append(actionOptions, aicommon.WithActionNonce(nonce))

	waitStream := utils.NewOnce()

	for _, _tagInstance := range tagFields.Values() {
		waitStream.Do(func() {
			streamWG.Add(1)
			actionOptions = append(actionOptions, aicommon.WithActionOnReaderFinished(func() {
				streamWG.Done()
			}))
		})

		v := _tagInstance

		actionOptions = append(actionOptions,
			aicommon.WithActionTagToKey(v.TagName, v.VariableName),
			aicommon.WithActionFieldStreamHandler([]string{v.VariableName}, func(key string, fieldReader io.Reader) {
				nodeId := v.AINodeId
				contentType := v.ContentType
				if nodeId == "" {
					nodeId = "re-act-loop-answer-payload"
				}

				if contentType == "" {
					contentType = "text/plain"
				}

				callbackStart := time.Now()
				var result bytes.Buffer
				fieldReader = io.TeeReader(utils.UTF8Reader(fieldReader), &result)
				wg := sync.WaitGroup{}
				wg.Add(1)
				emitter.EmitStreamEventWithContentType(
					nodeId, fieldReader, taskIndex, contentType,
					func() {
						defer wg.Done()
						// Use parseStart instead of callbackStart to measure the whole streaming process
						r.Set(v.VariableName, result.String())
						totalCost := time.Since(callbackStart)
						contentLength := len(result.String())
						log.Debugf("tag[%s] callback finished, content length: %d chars, total stream cost: %v",
							v.TagName, contentLength, totalCost)

						if totalCost.Milliseconds() <= 300 {
							log.Warnf("AITag[%s] stream too fast, cost %v (content: %d chars), stream maybe not valid",
								v.TagName, totalCost, contentLength)
						} else {
							log.Infof("AITag[%s] stream processing completed normally, cost %v for %d chars",
								v.TagName, totalCost, contentLength)
						}
					},
				)
				wg.Wait()
			}),
		)
	}
	return actionOptions
}

func (r *ReActLoop) Execute(taskId string, ctx context.Context, userInput string) error {
	task := aicommon.NewStatefulTaskBase(
		taskId,
		userInput,
		ctx,
		r.GetEmitter(),
	)

	if r.onTaskCreated != nil {
		r.onTaskCreated(task)
	}

	utils.Debug(func() {
		fmt.Println("---------------------------------------------")
		fmt.Println("start to handle userInput \n" + utils.PrefixLines(userInput, "> "))
		fmt.Println("---------------------------------------------")
	})
	defer func() {
		utils.Debug(func() {
			fmt.Println("---------------------------------------------")
			fmt.Println("end to handle userInput \n" + utils.PrefixLines(userInput, "> "))
			fmt.Println("---------------------------------------------")
		})
	}()
	err := r.ExecuteWithExistedTask(task)
	if task.IsAsyncMode() {
		return err
	}
	task.Finish(err)
	return err
}

func (r *ReActLoop) callAITransaction(streamWg *sync.WaitGroup, prompt string, nonce string) (*aicommon.Action, *LoopAction, error) {
	var action *aicommon.Action
	var emitter = r.emitter
	var actionNames = r.GetAllActionNames()

	getNextActionType := func(a *aicommon.Action) string { //legacy support
		actionType := action.ActionType()
		if actionType == "object" {
			actionType = action.GetString("next_action.type")
		}
		return actionType
	}

	ctxCanceled := utils.NewBool(false)
	if r.GetCurrentTask() != nil {
		select {
		case <-r.GetCurrentTask().GetContext().Done():
			ctxCanceled.SetTo(true)
		default:
		}
	}

	log.Infof("start to call aicommon.CallAITransaction in ReActLoop[%v]", r.loopName)
	r.loadingStatus("等待 AI 回应 / Waiting AI Respond...")
	var promptRefOnce sync.Once
	transactionErr := aicommon.CallAITransaction(
		r.config,
		prompt,
		r.config.CallAI,
		func(resp *aicommon.AIResponse) error {
			if ctxCanceled.IsSet() {
				return nil
			}
			stream := resp.GetOutputStreamReader(
				r.loopName,
				true,
				r.config.GetEmitter(),
			)
			tagOptions := r.buildActionTagOption(streamWg, resp.GetTaskIndex(), nonce)
			streamFields := r.streamFields.Copy()

			for _, i := range r.GetAllActions() {
				for _, field := range i.StreamFields {
					streamFields.Set(field.FieldName, field)
				}
			}
			var actionErr error
			options := append(tagOptions, aicommon.WithActionAlias(actionNames...),
				aicommon.WithActionFieldStreamHandler(
					streamFields.Keys(),
					func(key string, reader io.Reader) {
						streamWg.Add(1)
						doneOnce := utils.NewOnce()
						done := func() {
							doneOnce.Do(func() {
								log.Debugf("stream handler for field [%s] done, streamWg.Done() called", key)
								streamWg.Done()
							})
						}

						// Ensure done is always called even if something goes wrong
						defer func() {
							if rec := recover(); rec != nil {
								log.Errorf("stream handler for field [%s] panic recovered: %v", key, rec)
								done()
							}
						}()

						log.Debugf("stream handler started for field [%s]", key)
						r.loadingStatus(fmt.Sprintf("处理流字段 [%s] / Processing Stream Field [%s]", key, key))

						reader = utils.JSONStringReader(reader)
						fieldIns, ok := streamFields.Get(key)
						if !ok {
							log.Warnf("stream field [%s] not found in streamFields, skipping", key)
							done()
							return
						}

						pr, pw := utils.NewPipe()
						copyStartTime := time.Now()
						go func(field *LoopStreamField) {
							defer func() {
								pw.Close()
								log.Debugf("stream copy goroutine for field [%s] completed, took %v", key, time.Since(copyStartTime))
							}()
							if field.Prefix != "" {
								pw.WriteString(field.Prefix + ": ")
							}
							n, copyErr := io.Copy(pw, reader)
							if copyErr != nil {
								log.Warnf("stream copy for field [%s] error: %v (copied %d bytes)", key, copyErr, n)
							} else {
								log.Debugf("stream copy for field [%s] success, copied %d bytes", key, n)
							}
						}(fieldIns)

						defaultNodeId := "re-act-loop-thought"
						if fieldIns.AINodeId != "" {
							defaultNodeId = fieldIns.AINodeId
						}

						event, emitErr := emitter.EmitStreamEventWithContentType(
							defaultNodeId,
							pr,
							resp.GetTaskIndex(),
							fieldIns.ContentType,
							func() {
								log.Debugf("stream emit callback for field [%s] triggered", key)
								done()
							},
						)
						if emitErr != nil {
							log.Errorf("EmitStreamEvent for field [%s] failed: %v", key, emitErr)
							done() // Ensure done is called even on error
							return
						}

						// Emit prompt as reference material (only once per transaction)
						if event != nil && prompt != "" {
							promptRefOnce.Do(func() {
								streamId := event.GetContentJSONPath(`$.event_writer_id`)
								if streamId != "" {
									emitter.EmitTextReferenceMaterial(streamId, prompt)
								}
							})
						}
					}),
			)

			r.loadingStatus("解析 AI 响应中 / Parsing AI Response...")
			extractStart := time.Now()
			action, actionErr = aicommon.ExtractActionFromStream(
				r.currentTask.GetContext(),
				stream,
				"object",
				options...,
			)
			log.Infof("ExtractActionFromStream completed, took %v, error: %v", time.Since(extractStart), actionErr)

			if actionErr != nil {
				r.loadingStatus("解析响应失败 / Parse Response Failed")
				return utils.Wrap(actionErr, "failed to parse action")
			}
			actionType := getNextActionType(action)
			if actionType == "" {
				r.loadingStatus("动作类型为空 / Action Type Empty")
				return utils.Error("action type is empty")
			}

			r.loadingStatus(fmt.Sprintf("处理动作 [%s] / Processing Action [%s]", actionType, actionType))
			log.Infof("action type extracted: %s", actionType)

			verifier, err := r.GetActionHandler(actionType)
			if err != nil {
				r.GetInvoker().AddToTimeline("error", fmt.Sprintf("action[%s] GetActionHandler failed: %v\nIf you encounter this error, try another '@action' and retry.", actionType, err))
				return utils.Wrapf(err, "action[%s] GetActionHandler failed", actionType)
			}
			if utils.IsNil(verifier) {
				return utils.Errorf("action[%s] verifier is nil", actionType)
			}
			if verifier.ActionVerifier == nil {
				r.loadingStatus(fmt.Sprintf("动作 [%s] 验证跳过 / Action [%s] Verify Skipped", actionType, actionType))
				return nil
			}

			r.loadingStatus(fmt.Sprintf("验证动作 [%s] / Verifying Action [%s]", actionType, actionType))
			return verifier.ActionVerifier(r, action)
		},
	)
	if transactionErr != nil {
		r.loadingStatus(fmt.Sprintf("AI 事务失败 / AI Transaction Failed: %v", transactionErr))
		log.Errorf("AI transaction failed: %v", transactionErr)
		return nil, nil, transactionErr
	}

	if ctxCanceled.IsSet() {
		r.loadingStatus("任务上下文已取消 / Task Context Cancelled")
		return nil, nil, utils.Error("task context canceled before execute ReActLoop")
	}

	if utils.IsNil(action) {
		r.loadingStatus("动作解析为空 / Action is Nil")
		return nil, nil, utils.Error("action is nil in ReActLoop")
	}

	r.loadingStatus(fmt.Sprintf("动作解析完成 [%s] / Action Parsed [%s]", action.Name(), action.Name()))

	handler, err := r.GetActionHandler(getNextActionType(action))
	if err != nil {
		return nil, nil, utils.Wrap(err, "GetActionHandler failed")
	}
	if utils.IsNil(handler) {
		return nil, nil, utils.Errorf("action[%s] 's handler is nil in ReActLoop.actions", action.Name())
	}

	// Wait for all streams to complete with timeout (max 3 seconds)
	// Don't block forever if streams are stuck
	r.loadingStatus("等待流处理完成 / Waiting Streams to Complete...")
	log.Infof("action.WaitStream starting for action [%s] with 3s timeout", action.Name())
	waitStart := time.Now()

	// Create a timeout context for stream waiting
	streamWaitCtx, streamWaitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer streamWaitCancel()

	// Wait with timeout
	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		action.WaitStream(r.GetCurrentTask().GetContext())
	}()

	select {
	case <-waitDone:
		log.Infof("action.WaitStream completed normally for action [%s], took %v", action.Name(), time.Since(waitStart))
		r.loadingStatus("流处理完成 / Streams Completed")
	case <-streamWaitCtx.Done():
		log.Warnf("action.WaitStream timeout (3s) for action [%s], continuing execution", action.Name())
		r.loadingStatus("流处理超时,继续执行 / Stream Wait Timeout, Continuing...")
	}

	return action, handler, nil
}

const ReActLoadingStatusKey = "re-act-loading-status-key"

func (r *ReActLoop) loadingStatus(i string) {
	if r.emitter == nil {
		return
	}
	log.Infof("re-act-loop loading status updated: %v", i)
	r.emitter.EmitStatus(ReActLoadingStatusKey, i)
}

func (r *ReActLoop) LoadingStatus(i string) {
	if utils.IsNil(r) {
		return
	}
	r.loadingStatus(i)
}

func (r *ReActLoop) ExecuteWithExistedTask(task aicommon.AIStatefulTask) error {
	r.loadingStatus("初始化 / initializing...")
	defer r.loadingStatus("end")

	if utils.IsNil(task) {
		return errors.New("re-act loop task is nil")
	}

	if r == nil {
		return errors.New("re-act loop is nil")
	}
	if r.taskMutex == nil {
		return errors.New("re-act loop taskMutex is nil")
	}

	select {
	case <-task.GetContext().Done():
		return utils.Errorf("task context done before execute ReActLoop: %v", task.GetContext().Err())
	default:
	}

	r.SetCurrentTask(task)
	r.ensureLoopDirectory(task)

	// Initialize action constraints from init handler
	var initOperator *InitTaskOperator

	if r.initHandler != nil {
		r.loadingStatus("执行初始化函数 / execute init handler...")
		utils.Debug(func() {
			fmt.Println("================================================")
			fmt.Printf("re-act loop [%v] task init handler start to execute\n", r.loopName)
			fmt.Println("================================================")
		})

		initOperator = newInitTaskOperator()
		r.initHandler(r, task, initOperator)

		// Check operator status
		if initOperator.IsDone() {
			// Init handler completed the task, exit immediately (early routing)
			r.loadingStatus("init handler done (early exit)")
			log.Infof("ReactLoop[%v] init handler signaled Done, exiting early", r.loopName)
			r.GetInvoker().AddToTimeline("init_done", fmt.Sprintf("ReActLoop[%v] init handler completed task early", r.loopName))
			return nil
		}

		if failed, failErr := initOperator.IsFailed(); failed {
			r.loadingStatus("init handler failed: " + failErr.Error())
			inv := r.GetInvoker()
			inv.AddToTimeline("error", fmt.Sprintf("ReActLoop[%v] task init handler execute failed: %v", r.loopName, failErr))
			query := "Task initialization failed: " + failErr.Error() + "\n\n Origin INPUT: " + task.GetUserInput() + "\n\n Please give some practical advice for fix this issue or help user"
			ctx := inv.GetConfig().GetContext()
			if !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			result, err := inv.DirectlyAnswer(ctx, query, nil)
			if err != nil {
				return utils.Errorf("re-act loop [%v] task init handler execute failed: %v; additionally, failed to get direct answer: %v", r.loopName, failErr, err)
			}
			inv.EmitFileArtifactWithExt("init_error_advice.txt", ".md", result)
			return utils.Errorf("re-act loop [%v] task init handler execute failed: %v", r.loopName, failErr)
		}

		// Continue with normal execution
		r.loadingStatus("init handler done")

		// Apply action constraints from init handler
		if initOperator.HasActionConstraints() {
			r.initActionMustUse = initOperator.GetNextActionMustUse()
			r.initActionDisabled = initOperator.GetNextActionDisabled()
			r.initActionApplied = false // Will be applied in first iteration
			log.Infof("ReactLoop[%v] init set action constraints: must_use=%v, disabled=%v",
				r.loopName, r.initActionMustUse, r.initActionDisabled)
		}
	}

	done := utils.NewOnce()
	abort := func(err error) {
		result := task.GetResult()
		result += "\n\n[Error]: " + err.Error()
		task.SetResult(result)
		done.Do(func() {
			if !testIsFinished(task) {
				task.SetStatus(aicommon.AITaskState_Aborted)
			}
		})
	}
	complete := func(err any) {
		if !utils.IsNil(err) {
			result := task.GetResult()
			result += "\n\n[Reason]: " + utils.InterfaceToString(err)
			task.SetResult(result)
		}
		done.Do(func() {
			if task.GetStatus() == aicommon.AITaskState_Skipped {
				log.Infof("re-act loop [%v] task[%v] skipped", r.loopName, r.currentTask.GetId())
			} else {
				task.SetStatus(aicommon.AITaskState_Completed)
			}
		})
	}

	taskStartProcessing := func() {
		task.SetStatus(aicommon.AITaskState_Processing)
	}

	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			abort(utils.Errorf("ReActLoop panicked: %v", err))
		} else {
			complete(nil)
		}
	}()

	nonce := utils.RandStringBytes(4)
	_ = nonce

	var iterationCount int
	var maxIterations int
	if r.maxIterations > 0 {
		maxIterations = r.maxIterations
	} else {
		maxIterations = 100
	}
	var emitter = r.emitter
	if utils.IsNil(emitter) {
		abort(utils.Errorf("Emitter is nil"))
		return utils.Error("emitter is nil in ReActLoop")
	}

	if r.NoActions() {
		abort(utils.Errorf("no action names in ReActLoop"))
		return utils.Error("no action names in ReActLoop")
	}

	var operator = newLoopActionHandlerOperator(task)
	var finalError error
	defer func() {
		if finalError != nil {
			abort(finalError)
		} else {
			complete(nil)
		}
	}()

	taskStartProcessing()

	// Initialize timeline differ to track changes during this task execution
	// This captures the baseline BEFORE any task-related timeline entries are added
	// We get the timeline from the invoker's config
	if invoker := r.GetInvoker(); invoker != nil {
		if cfg := invoker.GetConfig(); cfg != nil {
			if configWithTimeline, ok := cfg.(*aicommon.Config); ok && configWithTimeline.Timeline != nil {
				r.timelineDiffer = aicommon.NewTimelineDiffer(configWithTimeline.Timeline)
				r.timelineDiffer.SetBaseline()
				log.Debugf("ReactLoop[%s] timeline baseline set, items: %d", r.loopName, configWithTimeline.Timeline.GetIdToTimelineItem().Len())
			}
		}
	}

	r.GetInvoker().AddToTimeline(aicommon.TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT, fmt.Sprintf("%v", task.GetOriginUserInput()))

	if r.GetCurrentMemoriesContent() == "" {
		r.fastLoadSearchMemoryWithoutAI(task.GetUserInput())
	}

	go func() {
		if !utils.IsNil(r.memoryTriage) {
			log.Info("start to handle searching memory for ReActLoop with AI")
			result, err := r.memoryTriage.SearchMemory(task.GetUserInput(), 5*1024)
			if err != nil {
				log.Warnf("search memory failed: %v", err)
			}
			r.PushMemory(result)
		}
	}()

	needSummary := utils.NewBool(false)
LOOP:
	for {
		iterationCount++
		if iterationCount > maxIterations {
			maxIterErr := utils.Errorf("reached max iterations (%d), stopping code generation loop", maxIterations)
			postOp := r.finishIterationLoopWithError(iterationCount, task, maxIterErr)

			// 检查 Hook 是否要求忽略错误
			if postOp.ShouldIgnoreError() {
				log.Infof("Loop exit with ignored error: max iterations reached (%d)", maxIterations)
				needSummary.SetTo(true)
				break LOOP // 正常退出，不返回错误
			}

			log.Warnf("Reached max iterations (%d), stopping code generation loop", maxIterations)
			needSummary.SetTo(true)
			break LOOP
		}

		waitMem := make(chan struct{})
		go func() {
			defer func() {
				close(waitMem)
			}()
			r.fastLoadSearchMemoryWithoutAI(task.GetUserInput())
		}()

		r.loadingStatus("记忆快速装载中 / waiting for fast memories to load...")
		select {
		case <-task.GetContext().Done():
			return utils.Errorf("task context done before execute ReActLoop: %v", task.GetContext().Err())
		case <-waitMem:
			r.loadingStatus("记忆已装载 / memories loaded")
		case <-time.After(200 * time.Millisecond):
			r.loadingStatus("跳过快速记忆装载，原因：超时 / skipping wait memories due to timeout")
		}

		r.loadingStatus("执行中... / executing...")
		var prompt string
		prompt, finalError = r.generateLoopPrompt(
			nonce,
			task.GetUserInput(),
			r.GetCurrentMemoriesContent(),
			operator,
		)
		if finalError != nil {
			r.finishIterationLoopWithError(iterationCount, task, finalError)
			log.Errorf("Failed to generate prompt: %v", finalError)
			needSummary.SetTo(true)
			return finalError
		}

		// Save prompt to file in debug mode
		if r.isDebugModeEnabled() {
			r.savePromptToFile(task, iterationCount, prompt)
		}

		streamWg := new(sync.WaitGroup)
		/* Generate AI Action */
		actionParams, handler, transactionErr := r.callAITransaction(streamWg, prompt, nonce)

		streamWg.Wait()

		if transactionErr != nil {
			r.finishIterationLoopWithError(iterationCount, task, transactionErr)
			log.Errorf("Failed to execute loop: %v", transactionErr)
			needSummary.SetTo(true)
			break LOOP
		}

		utils.Debug(func() {
			fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			fmt.Printf("AI decide to exec action[%v]: %v", actionParams.ActionType(), actionParams.GetParams().Dump())
			fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
		})

		if utils.IsNil(actionParams) {
			r.finishIterationLoopWithError(iterationCount, task, utils.Error("action is nil in ReActLoop"))
			log.Error("action is nil in ReActLoop")
			needSummary.SetTo(true)
			break LOOP
		}
		actionName := actionParams.Name()

		r.loadingStatus(fmt.Sprintf("[%v]执行中 / [%v] executing action...", actionName, actionName))

		// 记录当前迭代索引和 Action 信息
		r.actionHistoryMutex.Lock()
		r.currentIterationIndex = iterationCount
		actionRecord := &ActionRecord{
			ActionType:     actionParams.ActionType(),
			ActionName:     actionName,
			ActionParams:   make(map[string]interface{}),
			IterationIndex: iterationCount,
		}
		// 复制 Action 参数（避免并发修改）
		params := actionParams.GetParams()
		for k, v := range params {
			actionRecord.ActionParams[k] = v
		}
		r.actionHistory = append(r.actionHistory, actionRecord)
		r.actionHistoryMutex.Unlock()

		r.emitActionExecutionRecord(task, actionParams, iterationCount, prompt)

		// allow iteration info to be added to timeline
		loopName := r.loopName
		if loopName == "" {
			loopName = "general-purpose"
		}
		reason := actionParams.GetString("human_readable_thought")
		msg := fmt.Sprintf("[%v]======== ReAct iteration %d ========", loopName, iterationCount)
		if reason != "" {
			msg += "\nReason/Next-Step: " + reason
		}
		r.GetInvoker().AddToTimeline("iteration", msg)

		if handler.AsyncMode {
			task.SetAsyncMode(true)
			emitter.EmitJSON(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC, `react_task_mode_changed`, map[string]any{
				"task_id":         task.GetId(),
				"loop_name":       r.loopName,
				"task_index":      task.GetIndex(),
				"task_user_input": task.GetUserInput(),
			})

			// 异步模式不在主循环更新状态
			// 只能在异步回调中更新状态
			// 否则会出现状态被覆盖的问题
			if r.onAsyncTaskTrigger != nil {
				r.onAsyncTaskTrigger(handler, task)
			}
			done.Do(func() {
				log.Infof("async mode, not update task status in mainloop")
			})
		}

		// 重置上次操作状态对这次反应的影响
		operator = newLoopActionHandlerOperator(task)
		// 调用 ActionHandler
		if handler.ActionHandler == nil {
			// ActionHandler 必须存在
			finalError = utils.Errorf("action[%s] has no ActionHandler", actionName)
			r.finishIterationLoopWithError(iterationCount, task, finalError)
			needSummary.SetTo(true)
			return finalError
		}

		continueIter := func() {
			r.GetInvoker().AddToTimeline("iteration", fmt.Sprintf("[%v]ReAct Iteration Done[%v] max:%v continue to next iteration", loopName, iterationCount, maxIterations))
		}

		select {
		case <-task.GetContext().Done():
			return utils.Errorf("task context done in executing ReActLoop(before ActionHandler): %v", task.GetContext().Err())
		default:
		}

		// 记录 action 执行开始时间
		actionStartTime := time.Now()

		handler.ActionHandler(
			r,
			actionParams,
			operator,
		)

		// 计算 action 执行时间
		actionExecutionDuration := time.Since(actionStartTime)

		// 先检查 operator 状态，如果 operator 已经表明要终止（无论成功或失败），
		// 则 context canceled 不应该被视为错误
		// 这处理了 focus loop 正常完成后 context 被取消的情况
		if isTerminated, opErr := operator.IsTerminated(); isTerminated {
			// operator 已经决定终止，跳过 context canceled 检查
			log.Infof("ReactLoop[%v] terminated by operator after action execution", r.loopName)
			if opErr != nil {
				finalError = opErr
				utils.Debug(func() {
					fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
					fmt.Printf("[IsTerminated-Early] action executed[%v]: \n%v\npreparing for end iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
					fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				})
				r.finishIterationLoopWithError(iterationCount, task, finalError)
				return finalError
			}
			if !operator.isSilence {
				// 正常退出
				continueIter()
			}
			utils.Debug(func() {
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				fmt.Printf("[IsTerminated-Early] action executed[%v]: \n%v\npreparing for end iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			})
			r.finishIterationLoopWithError(iterationCount, task, nil)
			return nil
		}

		// 只有在 operator 没有明确终止时，才检查 context canceled
		select {
		case <-task.GetContext().Done():
			return utils.Errorf("task context done in executing execute ReActLoop(after ActionHandler): %v", task.GetContext().Err())
		default:
		}

		// 执行自我反思（如果启用）
		reflectionLevel := r.shouldTriggerReflection(handler, operator, iterationCount)
		if reflectionLevel != ReflectionLevel_None {
			r.loadingStatus(fmt.Sprintf("[%v]反思中 / [%v] self-reflecting...", actionName, actionName))
			log.Infof("trigger self-reflection for action[%s] with level[%s]", actionName, reflectionLevel.String())
			r.executeReflection(handler, actionParams, operator, reflectionLevel, iterationCount, actionExecutionDuration)
		}

		// 检查 operator 状态
		if isTerminated, err := operator.IsTerminated(); isTerminated {
			log.Infof("ReactLoop[%v] terminated", r.loopName)
			if err != nil {
				finalError = err
				utils.Debug(func() {
					fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
					fmt.Printf("[IsTerminated] action executed[%v]: \n%v\npreparing for end iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
					fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				})
				r.finishIterationLoopWithError(iterationCount, task, finalError)
				return finalError
			}
			if !operator.isSilence {
				// 正常退出
				continueIter()
			}
			utils.Debug(func() {
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				fmt.Printf("[IsTerminated] action executed[%v]: \n%v\npreparing for end iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			})
			r.finishIterationLoopWithError(iterationCount, task, nil)
			return nil
		}

		effectiveAsyncMode := handler.AsyncMode || operator.IsAsyncModeRequested()
		if effectiveAsyncMode {
			if !handler.AsyncMode {
				// dynamic async mode requested by handler at runtime
				task.SetAsyncMode(true)
				emitter.EmitJSON(schema.EVENT_TYPE_AI_TASK_SWITCHED_TO_ASYNC, `react_task_mode_changed`, map[string]any{
					"task_id":         task.GetId(),
					"loop_name":       r.loopName,
					"task_index":      task.GetIndex(),
					"task_user_input": task.GetUserInput(),
				})
				if r.onAsyncTaskTrigger != nil {
					r.onAsyncTaskTrigger(handler, task)
				}
				// Consume the done guard to prevent the deferred complete() from
				// prematurely marking the task as Completed while the async forge
				// is still running. This mirrors the static AsyncMode path (line 677).
				done.Do(func() {
					log.Infof("dynamic async mode, not update task status in mainloop")
				})
			}
			r.loadingStatus("当前任务进入异步模式 / Async mode, ending loop")
			finalError = nil
			utils.Debug(func() {
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				fmt.Printf("[Async] action executed[%v]: \n%v\npreparing for end iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			})
			r.finishIterationLoopWithError(iterationCount, task, finalError)
			return nil
		}

		// 非异步模式，继续下一次循环
		if operator.IsContinued() {
			continueIter()
			utils.Debug(func() {
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
				fmt.Printf("[Continue] action executed[%v]: \n%v\npreparing for next iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
				fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			})
			postOp := r.doneCurrentIteration(iterationCount, task)
			// Check if post-iteration callback requested to end the loop
			if postOp.ShouldEndIteration() {
				log.Infof("Loop ending due to post-iteration operator request: %v", postOp.GetEndReason())
				needSummary.SetTo(true)
				break LOOP
			}
			continue
		}

		// 如果既没有调用 Exit/Fail 也没有调用 Continue，默认继续
		continueIter()
		utils.Debug(func() {
			fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
			fmt.Printf("[Default Continue] action executed[%v]: \n%v\npreparing for next iteration\n", actionParams.ActionType(), actionParams.GetParams().Dump())
			fmt.Println("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<")
		})
		postOp := r.doneCurrentIteration(iterationCount, task)
		// Check if post-iteration callback requested to end the loop
		if postOp.ShouldEndIteration() {
			log.Infof("Loop ending due to post-iteration operator request: %v", postOp.GetEndReason())
			needSummary.SetTo(true)
			break LOOP
		}
		continue
	}
	return nil
}

func (r *ReActLoop) doneCurrentIteration(current int, task aicommon.AIStatefulTask) *OnPostIterationOperator {
	operator := newOnPostIterationOperator()
	if r.onPostIteration != nil {
		r.callOnPostIteration(current, task, false, nil, operator)
	}
	return operator
}

func (r *ReActLoop) callOnPostIteration(current int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
	// Phase 1: Run all registered callbacks in order.
	// Callbacks may set flags (e.g. IgnoreError()) and register deferred functions.
	for _, fn := range r.onPostIteration {
		fn(r, current, task, isDone, reason, operator)
	}
	// Phase 2: Run deferred functions after ALL callbacks have completed.
	// This ensures deferred logic can safely check the final operator state
	// (e.g. ShouldIgnoreError()) regardless of callback registration order.
	operator.RunDeferredFuncs()
}

func (r *ReActLoop) finishIterationLoopWithError(current int, task aicommon.AIStatefulTask, err any) *OnPostIterationOperator {
	operator := newOnPostIterationOperator()
	if r.onPostIteration != nil {
		if err != nil {
			r.callOnPostIteration(current, task, true, utils.Errorf("reason: %v", err), operator)
		} else {
			r.callOnPostIteration(current, task, true, nil, operator)
		}
	}
	return operator
}

func testIsFinished(task aicommon.AIStatefulTask) bool {
	return task.GetStatus() == aicommon.AITaskState_Completed || task.GetStatus() == aicommon.AITaskState_Aborted || task.GetStatus() == aicommon.AITaskState_Skipped
}

// ensureLoopDirectory initializes the loop directory metadata for artifact organization.
// It stores the task directory path and loop name prefix, which are used by
// GetLoopContentDir to construct flat content directories like:
//
//	task_{index}/loop_{name}_action_calls/
//	task_{index}/loop_{name}_prompts/
//	task_{index}/loop_{name}_data/
//
// This avoids the deep nesting of the old structure (task_{index}/loops/{name}/action_calls/).
func (r *ReActLoop) ensureLoopDirectory(task aicommon.AIStatefulTask) string {
	if utils.IsNil(r) || utils.IsNil(task) {
		return ""
	}
	workdir := r.config.GetOrCreateWorkDir()
	if workdir == "" {
		workdir = consts.GetDefaultBaseHomeDir()
	}
	taskIndex := task.GetIndex()
	if taskIndex == "" {
		taskIndex = "0"
	}
	loopName := r.loopName
	if loopName == "" {
		loopName = "unknown_loop"
	}

	taskSemanticId := task.GetSemanticIdentifier()
	taskDir := filepath.Join(workdir, aicommon.BuildTaskDirName(taskIndex, taskSemanticId))
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		log.Errorf("failed to create task directory %s: %v", taskDir, err)
		return ""
	}

	loopPrefix := "loop_" + sanitizeActionFilename(loopName)
	r.Set("task_directory", taskDir)
	r.Set("loop_name_prefix", loopPrefix)

	// For backward compatibility: "loop_directory" now points to the flat data directory
	// for callers that write files directly into the loop directory.
	loopDataDir := filepath.Join(taskDir, loopPrefix+"_data")
	r.Set("loop_directory", loopDataDir)
	return loopDataDir
}

// GetLoopContentDir returns a flat directory for a specific content type within the loop.
// Format: task_{index}/loop_{name}_{contentType}/
// Example: task_1-3/loop_default_action_calls/
//
// The directory is created if it does not exist. This method can be called by any code
// that needs to organize loop-specific artifacts into categorized flat directories.
func (r *ReActLoop) GetLoopContentDir(contentType string) string {
	taskDir := r.Get("task_directory")
	prefix := r.Get("loop_name_prefix")

	if taskDir == "" || prefix == "" {
		// Metadata not initialized yet; try to initialize from current task
		task := r.GetCurrentTask()
		if task == nil {
			return ""
		}
		r.ensureLoopDirectory(task)
		taskDir = r.Get("task_directory")
		prefix = r.Get("loop_name_prefix")
	}

	if taskDir == "" || prefix == "" {
		return ""
	}

	dir := filepath.Join(taskDir, prefix+"_"+contentType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Errorf("failed to create loop content directory %s: %v", dir, err)
		return ""
	}
	return dir
}

func (r *ReActLoop) savePromptToFile(task aicommon.AIStatefulTask, iteration int, prompt string) {
	if utils.IsNil(r) || utils.IsNil(task) {
		return
	}
	emitter := r.GetEmitter()
	if emitter == nil {
		return
	}

	// Use flat loop content directory: task_{index}/loop_{name}_prompts/
	promptDir := r.GetLoopContentDir("prompts")
	if promptDir == "" {
		log.Errorf("failed to get loop content directory for prompts")
		return
	}

	filename := fmt.Sprintf("iteration_%d_prompt_%d.md", iteration, time.Now().Unix())
	filePath := filepath.Join(promptDir, filename)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("# Iteration %d - Generated Prompt\n\n", iteration))
	content.WriteString(fmt.Sprintf("**Loop Name:** %s\n\n", r.loopName))
	content.WriteString(fmt.Sprintf("**Generated at:** %s\n\n", utils.DatetimePretty()))
	content.WriteString("---\n\n")
	content.WriteString(prompt)

	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		log.Errorf("failed to save prompt to file: %v", err)
		return
	}
	emitter.EmitPinFilename(filePath)
	log.Infof("saved prompt to file: %s", filePath)
}

func (r *ReActLoop) emitActionExecutionRecord(task aicommon.AIStatefulTask, action *aicommon.Action, iteration int, prompt string) {
	if utils.IsNil(r) || utils.IsNil(task) || utils.IsNil(action) {
		return
	}
	emitter := r.GetEmitter()
	if emitter == nil {
		return
	}

	// Use flat loop content directory: task_{index}/loop_{name}_action_calls/
	actionDir := r.GetLoopContentDir("action_calls")
	if actionDir == "" {
		log.Errorf("failed to get loop content directory for action_calls")
		return
	}

	actionName := action.Name()
	if actionName == "" {
		actionName = action.ActionType()
	}
	filename := fmt.Sprintf("%d_%s.md", iteration, sanitizeActionFilename(actionName))
	filePath := filepath.Join(actionDir, filename)

	content := r.buildActionExecutionMarkdown(actionName, action.GetParams(), action.GetString("human_readable_thought"), prompt, r.isDebugModeEnabled())
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		log.Errorf("failed to save action execution record to file: %v", err)
		return
	}
	emitter.EmitPinFilename(filePath)
	log.Infof("saved action execution record to file: %s", filePath)
}

func (r *ReActLoop) buildActionExecutionMarkdown(actionName string, params map[string]any, thought string, prompt string, includePrompt bool) string {
	var content strings.Builder
	content.WriteString("# Action Call Record\n\n")
	content.WriteString("## Action\n\n")
	content.WriteString("- Name: " + actionName + "\n")
	content.WriteString("- Human Readable Thought: " + thought + "\n\n")
	content.WriteString("## Params\n\n")
	content.WriteString("```json\n")
	content.WriteString(string(utils.Jsonify(params)))
	content.WriteString("\n```\n\n")
	if includePrompt {
		content.WriteString("## Prompt\n\n")
		content.WriteString("```\n")
		content.WriteString(prompt)
		content.WriteString("\n```\n")
	}
	return content.String()
}

func (r *ReActLoop) isDebugModeEnabled() bool {
	// Check debug_mode variable first
	value := r.GetVariable("debug_mode")
	if value != nil {
		if enabled, ok := value.(bool); ok && enabled {
			return true
		}
		if strings.EqualFold(utils.InterfaceToString(value), "true") {
			return true
		}
	}
	return false
}

func sanitizeActionFilename(name string) string {
	result := ""
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}
	if result == "" {
		return "unknown"
	}
	return result
}
