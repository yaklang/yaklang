package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
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

func (r *ReAct) executeMainLoop(userQuery string) (bool, error) {
	mainloop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_DEFAULT, r,
		reactloops.WithMemoryTriage(r.memoryTriage),
		reactloops.WithMemoryPool(r.memoryPool),
		reactloops.WithMemorySizeLimit(10*1024),
		reactloops.WithOnAsyncTaskTrigger(func(i *reactloops.LoopAction, task aicommon.AIStatefulTask) {
			r.SetCurrentPlanExecutionTask(task)
		}),
		reactloops.WithOnAsyncTaskFinished(func(task aicommon.AIStatefulTask) {
			r.SetCurrentPlanExecutionTask(nil)
		}),
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
			r.wg.Add(1)
			diffStr, err := r.timelineDiffer.Diff()
			if err != nil {
				log.Warnf("timeline differ call failed: %v", err)
				r.wg.Done()
				return
			}

			// 如果没有新的时间线差异，跳过记忆处理
			if diffStr == "" {
				if r.config.debugEvent {
					log.Infof("no timeline diff detected, skipping memory processing for iteration %d", iteration)
				}
				r.wg.Done()
				return
			}

			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("intelligent memory processing panic: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
					r.wg.Done()
				}()

				// 使用智能记忆处理系统
				if r.config.debugEvent {
					log.Infof("processing memory for iteration %d with timeline diff: %s", iteration, utils.ShrinkString(diffStr, 200))
				}

				// 构建上下文信息，包含任务状态和迭代信息
				contextualInput := fmt.Sprintf("ReAct迭代 %d/%s: %s\n任务状态: %s\n完成状态: %v\n原因: %v",
					iteration,
					task.GetId(),
					diffStr,
					string(task.GetStatus()),
					isDone,
					reason)

				// 使用HandleMemory进行智能记忆处理（包含去重、评分、保存）
				err := r.memoryTriage.HandleMemory(contextualInput)
				if err != nil {
					log.Warnf("intelligent memory processing failed: %v", err)
					return
				}

				if r.config.debugEvent {
					log.Infof("intelligent memory processing completed for iteration %d", iteration)
				}

				// 如果任务完成，尝试搜索相关记忆用于后续任务参考
				if isDone {
					go func() {
						defer func() {
							if err := recover(); err != nil {
								log.Errorf("memory search for completed task panic: %v", err)
								utils.PrintCurrentGoroutineRuntimeStack()
							}
						}()

						// 搜索与当前任务相关的记忆，限制在4KB内
						searchResult, err := r.memoryTriage.SearchMemory(task.GetUserInput(), 4096)
						if err != nil {
							log.Warnf("memory search for completed task failed: %v", err)
							return
						}

						if len(searchResult.Memories) > 0 {
							log.Infof("found %d relevant memories for completed task %s (total: %d bytes)",
								len(searchResult.Memories), task.GetId(), searchResult.ContentBytes)
							if r.config.debugEvent {
								log.Infof("memory search summary: %s", searchResult.SearchSummary)
								for i, mem := range searchResult.Memories {
									log.Infof("relevant memory %d: %s (tags: %v, relevance: C=%.2f, R=%.2f)",
										i+1, utils.ShrinkString(mem.Content, 100), mem.Tags, mem.C_Score, mem.R_Score)
								}
							}
						} else {
							if r.config.debugEvent {
								log.Infof("no relevant memories found for completed task %s", task.GetId())
							}
						}
					}()
				}
			}()
		}),
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
	return currentTask.IsAsyncMode(), nil
}
