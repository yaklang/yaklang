package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const recoveryTaskIDPrefix = "react-recovery-"

func formatRecoveryTaskID(coordinatorID string) string {
	return recoveryTaskIDPrefix + sanitizeForTaskId(coordinatorID) + uuid.New().String()
}

func newRecoveryPlanExecTask(ctx context.Context, emitter *aicommon.Emitter, coordinatorID string) aicommon.AIStatefulTask {
	return aicommon.NewStatefulTaskBase(
		formatRecoveryTaskID(coordinatorID),
		coordinatorID,
		ctx,
		emitter,
		true,
	)
}

func (r *ReAct) RequireAIForgeAndAsyncExecute(
	ctx context.Context, forgeName string,
	onFinished func(error),
) {

	doneOnce := utils.NewOnce()
	done := func(i error) {
		doneOnce.Do(func() {
			if onFinished != nil {
				onFinished(i)
			}
		})
	}

	// 验证 forgeName 不为空
	if forgeName == "" {
		errMsg := "AI Blueprint name is empty, cannot execute; AI 智能应用名称为空，无法执行。请指定正确的应用名称。"
		r.AddToTimeline("[BLUEPRINT_EMPTY_NAME]", errMsg)
		r.Emitter.EmitError(errMsg)
		done(utils.Error(errMsg))
		return
	}

	// 记录尝试调用 Blueprint
	r.AddToTimeline("[BLUEPRINT_INVOKE_START]", fmt.Sprintf("Invoking AI Blueprint: %s", forgeName))

	ins, forgeParams, err := r.invokeBlueprint(forgeName)
	if err != nil {
		// invokeBlueprint 已经记录了详细错误，这里只需要记录最终失败状态
		r.AddToTimeline("[BLUEPRINT_INVOKE_FAILED]", fmt.Sprintf("Failed to invoke '%s': %v", forgeName, err))
		r.Emitter.EmitError(fmt.Sprintf("Failed to invoke AI Blueprint '%s'", forgeName))
		// Merge result into timeline, do not emit result externally
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 调用失败，请检查应用名称和配置是否正确。错误详情：%v", forgeName, err))
		done(fmt.Errorf("failed to invoke ai-blueprint[%v]: %w", forgeName, err))
		return
	}

	// 再次验证返回的实例
	if ins == nil {
		r.AddToTimeline("[BLUEPRINT_NULL_AFTER_INVOKE]", fmt.Sprintf("AI Blueprint '%s' returned nil after invoke", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("AI Blueprint '%s' returned invalid instance", forgeName))
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 执行异常。", forgeName))
		done(utils.Error(fmt.Sprintf("AI Blueprint '%s' returned nil after successful invoke", forgeName)))
		return
	}

	forgeName = ins.ForgeName

	r.AddToTimeline("[BLUEPRINT_INVOKE_SUCCESS]", fmt.Sprintf("AI Blueprint '%s' (%s) ready with params: %v", forgeName, ins.ForgeVerboseName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256)))

	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")
	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			if finalError != nil {
				r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v finished with FAILURE: %v", utils.ShrinkString(forgeName, 128), finalError))
			} else {
				r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v is finished", utils.ShrinkString(forgeName, 128)))
			}
			r.emitArtifactsSummaryToTimeline()
			done(finalError)
		}()
		finalError = r.invokePlanAndExecute(taskDone, ctx, r.GetCurrentTask(), "", forgeName, forgeParams, "")
		if finalError != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan/forge: %v is started", utils.ShrinkString(forgeName, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinished func(error)) {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		finalError = r.invokePlanAndExecute(taskDone, ctx, r.GetCurrentTask(), planPayload, "", nil, "")
		if finalError != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan: %v is started", utils.ShrinkString(planPayload, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) AsyncRecoverPlanAndExecute(ctx context.Context, coordinatorID string, onFinished func(error)) {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	recoveryTask := newRecoveryPlanExecTask(ctx, r.Emitter, coordinatorID)
	r.addRuntimeTask(recoveryTask)

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan recovery: %v is finished", utils.ShrinkString(coordinatorID, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		finalError = r.invokePlanAndExecute(taskDone, recoveryTask.GetContext(), recoveryTask, "", "", nil, coordinatorID)
		if finalError != nil {
			log.Errorf("AsyncRecoverPlanAndExecute error: %v", finalError)
			recoveryTask.SetStatus(aicommon.AITaskState_Aborted)
			r.AddToTimeline("error", fmt.Sprintf("recovery task execution failed: %v", finalError))
		} else {
			recoveryTask.SetStatus(aicommon.AITaskState_Completed)
			r.AddToTimeline("success", "recovery task execution succeeded")
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan recovery: %v is started", utils.ShrinkString(coordinatorID, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) invokePlanAndExecute(doneChannel chan struct{}, ctx context.Context, task aicommon.AIStatefulTask, planPayload string, forgeName string, forgeParams any, coordinatorID string) (finalErr error) {
	doneOnce := new(sync.Once)
	done := func() {
		doneOnce.Do(func() {
			close(doneChannel)
		})
	}
	defer func() {
		done()
		if err := recover(); err != nil {
			log.Errorf("invokePlanAndExecute panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	defer func() {
		task.CallAsyncDeferCallback(finalErr)
	}()

	// create config with timeline
	// generate config
	uid := coordinatorID
	if uid == "" {
		uid = uuid.New().String()
	}
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	params := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": uid,
	}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, r.config.Id, params)
	defer func() {
		if finalErr != nil {
			r.EmitPlanExecFail(finalErr.Error())
		}
		r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, params)
	}()
	r.EmitAction(fmt.Sprintf("Plan request: %s", planPayload))

	if ctx == nil {
		ctx = r.config.Ctx
	}
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Preserve user original input in plan payload (mirrors forge branch logic)
	// AI-rewritten plan_request_payload may lose details like file paths
	if planPayload != "" {
		if task != nil {
			userOriginalInput := task.GetUserInput()
			if userOriginalInput != "" && !strings.Contains(planPayload, userOriginalInput) {
				nonce := utils.RandStringBytes(4)
				planPayload = utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .UserOriginalInput }}
<|用户原始需求_END_{{.nonce}}|>
---
{{ .PlanPayload }}
`,
					map[string]any{
						"nonce":             nonce,
						"UserOriginalInput": userOriginalInput,
						"PlanPayload":       planPayload,
					})
				log.Infof("enhanced plan payload with user original input to preserve context")
			}
		}
	}

	// if hijackPlanRequest is set, use it to handle the plan request
	// this is useful for testing/mocking and advanced usage
	if r.config.HijackPERequest != nil {
		r.EmitAction("hijack plan and execute in re-act mode")
		var payload string
		if planPayload == "" {
			payload = utils.InterfaceToString(forgeParams)
		} else {
			payload = planPayload
		}
		log.Infof("hijack plan and execute in re-act mode with payload: %v", utils.ShrinkString(planPayload, 200))
		done()
		return r.config.HijackPERequest(planCtx, payload)
	}

	inputChannel := chanx.NewUnlimitedChan[*ypb.AIInputEvent](r.config.Ctx, 10)
	r.config.InputEventManager.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			switch event.SyncType {
			case SYNC_TYPE_QUEUE_INFO:
				log.Infof("Received queue info sync event, ignoring in plan execution mode")
				return
			case aicommon.SYNC_TYPE_USER_INTERVENTION: // 临时方案
				log.Infof("Received user intervention event: %v", event)
				// warning not mirror user intervention events to timeline to avoid confusion
				return
			default:
				log.Infof("InvokePlanAndExecute: Received AI input event: %v", event)
			}
			inputChannel.SafeFeed(event)
		}()
	})
	defer func() {
		r.config.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
	}()

	hotpatchChan := r.config.HotPatchBroadcaster.Subscribe()
	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts,
		aicommon.WithID(uid),
		aicommon.WithTimeline(r.config.Timeline),
		func(cfg *aicommon.Config) error {
			// Preserve the parent's full callback set for delegated plan/forge execution.
			// Using only WithAICallback(original) would collapse quality/speed routing
			// into a single callback and can make delegated tasks drift to a different model.
			cfg.OriginalAICallback = r.config.OriginalAICallback
			cfg.QualityPriorityAICallback = r.config.QualityPriorityAICallback
			cfg.SpeedPriorityAICallback = r.config.SpeedPriorityAICallback
			cfg.AiServerName = r.config.AiServerName
			cfg.AiModelName = r.config.AiModelName
			return nil
		},
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithContext(planCtx),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			r.config.EventHandler(e)
		}),
	)

	if forgeName != "" {
		var opts = make([]aicommon.ConfigOption, len(baseOpts))
		for i, o := range baseOpts {
			opts[i] = o
		}
		stdOut := new(bytes.Buffer)
		eventHandler := func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			if e.Type == schema.EVENT_TYPE_YAKIT_EXEC_RESULT && e.IsJson {
				var execResult ypb.ExecResult
				if err := json.Unmarshal(e.Content, &execResult); err != nil {
					log.Errorf("Failed to unmarshal exec result: %v", err)
					return
				}
				if execResult.IsMessage {
					var yakitMsg yaklib.YakitMessage
					if err := json.Unmarshal(execResult.Message, &yakitMsg); err != nil {
						log.Errorf("Failed to unmarshal yakit message: %v", err)
						return
					}
					if yakitMsg.Type == "log" {
						var yakitLog yaklib.YakitLog
						if err := json.Unmarshal(yakitMsg.Content, &yakitLog); err != nil {
							log.Errorf("Failed to unmarshal yakit message: %v", err)
							return
						}
						stdOut.WriteString(yakitLog.String())
					}
				}
			}
			// Fix: Use EventHandler instead of Emit to avoid duplicate event saving
			// Events are already saved by the emitter's baseEmitter before EventHandler is called
			r.config.EventHandler(e)
		}
		opts = append(opts, aicommon.WithEventHandler(eventHandler))

		// Ensure user original input is preserved in forge parameters
		// This prevents context loss when AI rewrites the query parameter
		if task != nil {
			userOriginalInput := task.GetUserInput()
			if userOriginalInput != "" && forgeParams != nil {
				// Check if forgeParams contains user original input
				forgeParamsStr := utils.InterfaceToString(forgeParams)
				if !strings.Contains(forgeParamsStr, userOriginalInput) {
					// User original input is not in forge params, need to append it
					log.Infof("user original input not found in forge params, appending it to preserve context")

					// Try to modify forgeParams map if it's a map
					if paramsMap, ok := forgeParams.(map[string]any); ok {
						// Add user original input as a separate field
						nonce := utils.RandStringBytes(4)
						paramsMap["user_original_query"] = userOriginalInput

						// If there's a "query" field, enhance it with user original input
						if queryVal, exists := paramsMap["query"]; exists {
							queryStr := utils.InterfaceToString(queryVal)
							enhancedQuery := utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .UserOriginalInput }}
<|用户原始需求_END_{{.nonce}}|>
--- 
{{ .AIGeneratedQuery }}
`,
								map[string]any{
									"nonce":             nonce,
									"UserOriginalInput": userOriginalInput,
									"AIGeneratedQuery":  queryStr,
								})
							paramsMap["query"] = enhancedQuery
							log.Infof("enhanced forge query param with user original input")
						}
					}
				}
			}
		}

		done()
		result, err := aicommon.ExecuteForgeFromDB(forgeName, ctx, forgeParams, opts...)
		if err != nil {
			log.Errorf("Failed to execute forge: %v", err)
			return utils.Errorf("failed to execute forge %s: %v", forgeName, err)
		}
		_ = result
		r.AddToTimeline("forge output log", stdOut.String())
		r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)
		return nil
	} else {
		cod, err := aid.NewCoordinatorContext(planCtx, planPayload, baseOpts...)
		if err != nil {
			log.Errorf("Failed to create coordinator for plan execution: %v", err)
			return utils.Errorf("failed to create coordinator for plan execution: %v", err)
		}

		done()
		if err := cod.Run(); err != nil {
			log.Errorf("Plan execution failed: %v", err)
			return utils.Errorf("plan execution failed: %v", err)
		}
		r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)
		return nil
	}
}
