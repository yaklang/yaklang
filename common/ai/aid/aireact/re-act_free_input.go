package aireact

import (
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (r *ReAct) handleFreeValue(event *ypb.AIInputEvent) error {
	userInput := event.FreeInput
	if userInput == "" || strings.TrimSpace(userInput) == "" {
		return utils.Errorf("user input cannot be empty")
	}
	if r.config.debugEvent {
		log.Infof("Using free input: %s", userInput)
	}
	// Reset session state if needed
	r.finished = false
	r.currentIteration = 0
	if r.config.debugEvent {
		log.Infof("Reset ReAct session for new input")
	}
	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.enqueueTask(event)
}

// enqueueTask 将输入事件转换为任务并添加到队列
func (r *ReAct) enqueueTask(event *ypb.AIInputEvent) error {
	// 创建基于aireact.Task的任务（初始状态为created）
	task := NewTask(fmt.Sprintf("react-task-%v", ksuid.New().String()), event.FreeInput, r.Emitter)

	if r.config.debugEvent {
		log.Infof("Task created: %s with input: %s", task.GetId(), event.FreeInput)
	}

	r.queueMutex.Lock()
	defer r.queueMutex.Unlock()

	isCurrentlyProcessing := r.isProcessing
	currentTask := r.currentTask
	if !isCurrentlyProcessing {
		// 没有任务在处理，直接开始处理新任务
		task.SetStatus(string(TaskStatus_Processing))
		r.addToTimeline("processing", fmt.Sprintf("Task immediately started processing: %s", event.FreeInput), task.GetId())

		// 设置当前任务并标记为正在处理
		r.currentTask = task
		r.isProcessing = true

		if r.config.debugEvent {
			log.Infof("Task %s immediately started processing", task.GetId())
		}

		// 异步处理任务
		go r.processTask(task)
		return nil
	}

	log.Infof("Task enqueue started processing: %s", task.GetId())
	// 有任务正在处理，需要评估新任务是否与当前任务相关
	if currentTask != nil && task.IsRelatedTo(currentTask) {
		// 任务相关，进入evaluating状态然后直接追加到timeline
		task.SetStatus(string(TaskStatus_Evaluating))
		r.addToTimeline("evaluating", fmt.Sprintf("Task is related to current task, evaluating: %s", event.FreeInput), task.GetId())

		if r.config.debugEvent {
			log.Infof("Task %s is related to current task %s, adding context", task.GetId(), currentTask.GetId())
		}

		// 直接将相关信息追加到时间线作为上下文补充
		r.addToTimeline("context_supplement", fmt.Sprintf("Related input from task %s: %s", task.GetId(), event.FreeInput), currentTask.GetId())

		// 标记任务为已完成（作为上下文补充）
		task.SetStatus(string(TaskStatus_Completed))
		r.addToTimeline("completed", fmt.Sprintf("Task completed as context supplement: %s", event.FreeInput), task.GetId())

		return nil
	}

	// 任务不相关，进入排队状态
	task.SetStatus(string(TaskStatus_Queueing))
	r.addToTimeline("queueing", fmt.Sprintf("Task queued for later processing: %s", event.FreeInput), task.GetId())

	// 添加到队列
	err := r.taskQueue.Append(task)
	if err != nil {
		log.Errorf("Failed to add task to queue: %v", err)
		return fmt.Errorf("failed to enqueue task: %v", err)
	}

	if r.config.debugEvent {
		log.Infof("Task enqueued: %s with input: %s", task.GetId(), event.FreeInput)
	}

	return nil
}
