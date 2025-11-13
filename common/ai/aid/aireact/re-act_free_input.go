package aireact

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) handleFreeValue(event *ypb.AIInputEvent) error {
	userInput := event.FreeInput
	if userInput == "" || strings.TrimSpace(userInput) == "" {
		return utils.Errorf("user input cannot be empty")
	}
	if r.config.DebugEvent {
		log.Infof("Using free input: %s", userInput)
	}
	// Reset session state if needed
	r.currentIteration = 0
	if r.config.DebugEvent {
		log.Infof("Reset ReAct session for new input")
	}
	// Execute the main ReAct loop using the new schema-based approach
	if r.config.DebugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.enqueueReTask(event)
}

func (r *ReAct) setCurrentTask(task aicommon.AIStatefulTask) {
	r.lastTask = r.currentTask

	r.currentTask = task
	if r.config.DebugEvent {
		if task != nil {
			log.Infof("Current task set to: %s", task.GetId())
		}
	}
}

func (r *ReAct) IsProcessingReAct() bool {
	return r.currentTask != nil
}

func (r *ReAct) GetRisks() []*schema.Risk {
	events, err := yakit.QueryAIEvent(r.config.GetDB(), &ypb.AIEventFilter{
		TaskIndex: []string{r.lastTask.GetId()},
	})
	if err != nil {
		return nil
	}

	risks := []*schema.Risk{}
	for _, event := range events {
		if event.Type == schema.EVENT_TYPE_YAKIT_RISK {
			riskInfo := map[string]any{}
			err := json.Unmarshal(event.Content, &riskInfo)
			if err != nil {
				continue
			}
			riskId, ok := riskInfo["risk_id"]
			if ok && riskId != nil {
				id := utils.InterfaceToInt(riskId)
				risk, err := yakit.GetRisk(r.config.GetDB(), int64(id))
				if err != nil {
					continue
				}
				risks = append(risks, risk)
			}
		}
	}
	return risks
}

func (r *ReAct) GetLastTask() aicommon.AIStatefulTask {
	if r.lastTask == nil {
		return nil
	}
	if r.config.DebugEvent {
		log.Infof("Last task retrieved: %s", r.lastTask.GetId())
	}
	return r.lastTask
}

func (r *ReAct) GetCurrentTask() aicommon.AIStatefulTask {
	if r.currentTask == nil {
		return nil
	}
	if r.config.DebugEvent {
		log.Infof("Current task retrieved: %s", r.currentTask.GetId())
	}
	return r.currentTask
}

func (r *ReAct) GetCurrentLoop() *reactloops.ReActLoop {
	currentTask := r.GetCurrentTask()
	if currentTask == nil {
		return nil
	}
	currentLoop := currentTask.GetReActLoop().(*reactloops.ReActLoop)
	if currentLoop == nil {
		return nil
	}
	return currentLoop
}

func (r *ReAct) DumpCurrentEnhanceData() string {
	if r.config.EnhanceKnowledgeManager == nil {
		return ""
	}
	data := r.config.EnhanceKnowledgeManager.DumpTaskAboutKnowledge(r.GetCurrentTask().GetId())
	if r.config.DebugEvent {
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
	if r.config.DebugEvent {
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
	if r.config.DebugEvent {
		log.Infof("Task enqueued: %s with input: %s", task.GetId(), event.FreeInput)
	}
	return nil
}
