package aid

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (c *Coordinator) ExecuteLoopTask(taskTypeName string, task aicommon.AIStatefulTask, options ...reactloops.ReActLoopOption) error {
	memoryFlushBuffer := aicommon.NewMemoryFlushBuffer("coordinator", c.TimelineDiffer, nil)
	defer memoryFlushBuffer.Close()
	taskCtx := task.GetContext()
	inputChannel := chanx.NewUnlimitedChan[*ypb.AIInputEvent](taskCtx, 10)
	uid := uuid.NewString()
	c.InputEventManager.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			switch event.SyncType {
			case "queue_info":
			default:
				log.Infof("Coordinator: Received AI input event: %v", event)
			}
			inputChannel.SafeFeed(event)
		}()
	})
	defer func() {
		c.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
	}()
	ctx, cancel := context.WithCancel(taskCtx)
	defer cancel()
	hotpatchChan := c.Config.HotPatchBroadcaster.Subscribe()
	baseOpts := aicommon.ConvertConfigToOptions(c.Config)
	baseOpts = append(baseOpts,
		aicommon.WithID(c.Config.Id), // pe -> react should use same id
		aicommon.WithAutoTieredAICallback(c.OriginalAICallback),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithContext(ctx),
		aicommon.WithConsumption(c.GetConsumptionConfig()),
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
	)

	invoker, err := aicommon.AIRuntimeInvokerGetter(c.GetContext(), baseOpts...)
	if err != nil {
		return fmt.Errorf("创建 AI 调用运行时失败: %v", err)
	}

	defaultOptions := []reactloops.ReActLoopOption{
		reactloops.WithMemoryTriage(c.MemoryTriage),
		reactloops.WithMemoryPool(c.MemoryPool),
		reactloops.WithMemorySizeLimit(int(c.MemoryPoolSize)),
		reactloops.WithEnableSelfReflection(c.EnableSelfReflection),
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
			operator.DeferAfterCallbacks(func() {
				if c.MemoryTriage == nil {
					return
				}
				memoryFlushBuffer.ProcessAsync(aicommon.MemoryFlushSignal{
					Iteration:          iteration,
					Task:               task,
					IsDone:             isDone,
					Reason:             reason,
					ShouldEndIteration: operator.ShouldEndIteration(),
				}, func(payload *aicommon.MemoryFlushPayload, err error) {
					if err != nil {
						log.Warnf("timeline differ call failed: %v", err)
						return
					}
					if payload == nil && !isDone {
						return
					}

					go func() {
						defer func() {
							if err := recover(); err != nil {
								log.Errorf("intelligent memory processing panic: %v", err)
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}()

						if payload != nil {
							if c.Config.DebugEvent {
								log.Infof("processing memory flush[%s] for iteration %d with %d pending diffs (%d bytes)", payload.FlushReason, iteration, payload.PendingIterations, payload.PendingBytes)
							}
							if err := c.MemoryTriage.HandleMemory(payload.ContextualInput); err != nil {
								log.Warnf("intelligent memory processing failed: %v", err)
								return
							}
						}

						if isDone && !task.IsAsyncMode() {
							searchResult, err := c.MemoryTriage.SearchMemory(task.GetUserInput(), 4096)
							if err != nil {
								log.Warnf("memory search for completed task failed: %v", err)
								return
							}
							if len(searchResult.Memories) > 0 {
								log.Infof("found %d relevant memories for completed task %s (total: %d bytes)", len(searchResult.Memories), task.GetId(), searchResult.ContentBytes)
								if c.DebugEvent {
									log.Infof("memory search summary: %s", searchResult.SearchSummary)
								}
							} else if c.DebugEvent {
								log.Infof("no relevant memories found for completed task %s", task.GetId())
							}
						}
					}()
				})
			})
		}),
	}

	defaultOptions = append(defaultOptions, options...)

	mainloop, err := reactloops.CreateLoopByName(
		taskTypeName, invoker,
		defaultOptions...,
	)
	if err != nil {
		return utils.Errorf("failed to create main loop runtime instance: %v", err)
	}
	mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
	mainloop.RemoveAction(schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
	task.SetAsyncMode(false)
	invoker.SetCurrentTask(task)
	err = mainloop.ExecuteWithExistedTask(task)
	if err != nil {
		return err
	}
	return nil
}
