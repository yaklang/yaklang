package aireact

import (
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	r.currentIteration = 0
	if r.config.debugEvent {
		log.Infof("Reset ReAct session for new input")
	}
	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.enqueueReTask(event)
}

func (r *ReAct) setCurrentTask(task aicommon.AIStatefulTask) {
	r.currentTask = task
	if r.config.debugEvent {
		if task != nil {
			log.Infof("Current task set to: %s", task.GetId())
		}
	}
}

func (r *ReAct) IsProcessingReAct() bool {
	return r.currentTask != nil
}

func (r *ReAct) GetCurrentTask() aicommon.AIStatefulTask {
	if r.currentTask == nil {
		return nil
	}
	if r.config.debugEvent {
		log.Infof("Current task retrieved: %s", r.currentTask.GetId())
	}
	return r.currentTask
}

func (r *ReAct) DumpCurrentEnhanceData() string {
	if r.config.enhanceKnowledgeManager == nil {
		return ""
	}
	data := r.config.enhanceKnowledgeManager.DumpTaskAboutKnowledge(r.GetCurrentTask().GetId())
	if r.config.debugEvent {
		log.Infof("Dumped enhance data: %s", data)
	}
	return data
}

// enqueueReTask 将输入事件转换为任务并添加到队列
func (r *ReAct) enqueueReTask(event *ypb.AIInputEvent) error {
	// 创建基于aireact.Task的任务（初始状态为created）
	task := aicommon.NewStatefulTaskBase(
		fmt.Sprintf("re-act-task-%v", ksuid.New().String()),
		event.FreeInput,
		r.config.GetContext(),
		r.Emitter)
	if r.config.debugEvent {
		log.Infof("Task created: %s with input: %s", task.GetId(), event.FreeInput)
	}

	log.Infof("Task enqueue started processing: %s", task.GetId())
	// 任务不相关，进入排队状态
	task.SetStatus(aicommon.AITaskState_Queueing)
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
