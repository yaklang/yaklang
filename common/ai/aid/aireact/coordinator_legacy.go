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
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak/yaklib"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// This channel only constructs and executes the unchanged legacy aid runtime.
func (r *ReAct) invokeLegacyPlanAndExecute(doneChannel chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) (finalErr error) {
	cfg := newInvokePlanAndExecuteOptions(opts...)
	task := cfg.task
	planPayload := cfg.planPayload
	if planPayload == "" && cfg.executePlanInput != nil {
		planPayload = cfg.executePlanInput.PlanPayload
	}
	forgeName := cfg.forgeName
	forgeParams := cfg.forgeParams
	coordinatorID := cfg.coordinatorID
	startTaskID := cfg.startTaskID

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
		"start_task_id":  startTaskID,
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
			case aicommon.SYNC_TYPE_USER_INTERVENTION, aicommon.SYNC_TYPE_RECOVERY_HISTORY: // 临时方案
				log.Infof("Received user covery history or intervention event: %v", event)
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
	defer r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)
	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts,
		aicommon.WithLiteForgeExecutor(nil), // Legacy owns its original helper channel, including on recovery.
		aicommon.WithID(uid),
		aicommon.WithTimeline(r.config.Timeline),
		aicommon.WithAICallbacks(r.config.GetRawAICallbacks()),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithContext(planCtx),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			r.config.EventHandler(e)
		}),
	)
	if startTaskID != "" {
		baseOpts = append(baseOpts, coordinator_legacy.WithRecoveryStartTaskID(startTaskID))
	}
	baseOpts = appendApprovedPlanArtifactOptions(baseOpts, cfg.executePlanInput)

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
		return nil
	} else {
		cod, err := newCoordinatorContextForPlanExec(planCtx, planPayload, baseOpts...)
		if err != nil {
			log.Errorf("Failed to create coordinator for plan execution: %v", err)
			return utils.Errorf("failed to create coordinator for plan execution: %v", err)
		}

		done()
		run := runCoordinatorForPlanExec
		if cfg.executePlanInput != nil {
			root, err := cod.BuildRootTaskFromPlanData(cfg.executePlanInput.PlanData, planPayload)
			if err != nil {
				return utils.Errorf("invalid approved plan: %w", err)
			}
			if err := cod.CommitApprovedPlan(root, cfg.executePlanInput.PlanDocument); err != nil {
				return err
			}
			run = runCoordinatorForExecuteApprovedPlan
		}
		if err := run(cod); err != nil {
			log.Errorf("Plan execution failed: %v", err)
			return utils.Errorf("plan execution failed: %v", err)
		}
		return nil
	}
}
