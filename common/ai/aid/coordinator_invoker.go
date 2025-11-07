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
	inputChannel := chanx.NewUnlimitedChan[*ypb.AIInputEvent](c.Ctx, 10)
	uid := uuid.NewString()
	c.InputEventManager.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			log.Infof("Received AI input event: %v", event)
			inputChannel.SafeFeed(event)
		}()
	})
	defer func() {
		c.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
	}()
	ctx, cancel := context.WithCancel(c.Ctx)
	defer cancel()
	baseOpts := aicommon.ConvertConfigToOptions(c.Config)
	baseOpts = append(baseOpts,
		aicommon.WithID(c.Config.Id), // pe -> react should use same id
		aicommon.WithWrapperedAICallback(c.QualityPriorityAICallback),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithContext(ctx),
		aicommon.WithConsumption(c.GetConsumptionConfig()),
		aicommon.WithEnablePlanAndExec(false),
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
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
			c.Add(1)
			diffStr, err := c.TimelineDiffer.Diff()
			if err != nil {
				log.Warnf("timeline differ call failed: %v", err)
				c.Done()
				return
			}

			// 如果没有新的时间线差异，跳过记忆处理
			if diffStr == "" {
				if c.DebugEvent {
					log.Infof("no timeline diff detected, skipping memory processing for iteration %d", iteration)
				}
				c.Done()
				return
			}

			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("intelligent memory processing panic: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
					c.Done()
				}()

				// 使用智能记忆处理系统
				if c.Config.DebugEvent {
					log.Infof("processing memory for iteration %d with timeline diff: %s", iteration, utils.ShrinkString(diffStr, 200))
				}

				contextualInput := fmt.Sprintf("ReAct迭代 %d/%s: %s\n任务状态: %s\n完成状态: %v\n原因: %v",
					iteration,
					task.GetId(),
					diffStr,
					string(task.GetStatus()),
					isDone,
					reason)

				err := c.MemoryTriage.HandleMemory(contextualInput)
				if err != nil {
					log.Warnf("intelligent memory processing failed: %v", err)
					return
				}

				if c.DebugEvent {
					log.Infof("intelligent memory processing completed for iteration %d", iteration)
				}

				if isDone {
					go func() {
						defer func() {
							if err := recover(); err != nil {
								log.Errorf("memory search for completed task panic: %v", err)
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}()

						// 搜索与当前任务相关的记忆，限制在4KB内
						searchResult, err := c.MemoryTriage.SearchMemory(task.GetUserInput(), 4096)
						if err != nil {
							log.Warnf("memory search for completed task failed: %v", err)
							return
						}

						if len(searchResult.Memories) > 0 {
							log.Infof("found %d relevant memories for completed task %s (total: %d bytes)",
								len(searchResult.Memories), task.GetId(), searchResult.ContentBytes)
							if c.DebugEvent {
								log.Infof("memory search summary: %s", searchResult.SearchSummary)
								for i, mem := range searchResult.Memories {
									log.Infof("relevant memory %d: %s (tags: %v, relevance: C=%.2f, R=%.2f)",
										i+1, utils.ShrinkString(mem.Content, 100), mem.Tags, mem.C_Score, mem.R_Score)
								}
							}
						} else {
							if c.DebugEvent {
								log.Infof("no relevant memories found for completed task %s", task.GetId())
							}
						}
					}()
				}
			}()
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
