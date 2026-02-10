package aireact

import (
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) handleFreeValue(event *ypb.AIInputEvent) error {
	if r.pureInvokerMode {
		return utils.Errorf("use in prue invoker mode, cannot handle free input")
	}
	userInput := event.FreeInput
	if userInput == "" || strings.TrimSpace(userInput) == "" {
		return utils.Errorf("user input cannot be empty")
	}
	for _, path := range event.AttachedFilePath {
		r.config.ContextProviderManager.RegisterTracedContent(path, aicommon.FileContextProvider(path, userInput))
	}
	// 现已经被 knowledge enhance loop 替代
	// for _, resource := range event.AttachedResourceInfo {
	// 	registrationKey := resource.GetType() + "_" + resource.GetKey() + resource.GetValue()
	// 	hashKey := codec.Md5(registrationKey)
	// 	r.config.ContextProviderManager.RegisterTracedContent(hashKey, aicommon.NewContextProvider(resource.GetType(), resource.GetKey(), resource.GetValue(), userInput))
	// }

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

func (r *ReAct) addRuntimeTask(task aicommon.AIStatefulTask) {
	r.UpdateRuntimeTaskMutex.Lock()
	defer r.UpdateRuntimeTaskMutex.Unlock()
	r.RuntimeTasks = append(r.RuntimeTasks, task)
	if r.config.DebugEvent {
		log.Infof("Runtime task added: %s", task.GetId())
	}
}

func (r *ReAct) setCurrentTask(task aicommon.AIStatefulTask) {
	r.lastTask = r.currentTask

	r.currentTask = task
	if r.currentTask != nil {
		r.currentTask.SetDB(r.config.GetDB())
	}
	if r.config.DebugEvent {
		if task != nil {
			log.Infof("Current task set to: %s", task.GetId())
		}
	}
}

func (r *ReAct) IsProcessingReAct() bool {
	return r.currentTask != nil
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

func (r *ReAct) GetCurrentTaskId() string {
	currentTask := r.GetCurrentTask()
	if currentTask == nil {
		return ""
	}
	return currentTask.GetId()
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

// sanitizeForTaskId extracts a meaningful prefix from user input for use in task IDs.
// Keeps ASCII letters, digits, underscores, Chinese characters. Truncates to 30 chars.
func sanitizeForTaskId(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "task"
	}
	var result []rune
	for _, r := range strings.ToLower(input) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result = append(result, r)
		} else if r == ' ' || r == '-' {
			result = append(result, '_')
		}
		// skip other characters (including Chinese to keep IDs filesystem-safe)
	}
	s := string(result)
	// collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	s = strings.Trim(s, "_")
	if len(s) > 30 {
		s = s[:30]
	}
	if s == "" {
		return "task"
	}
	return s
}

// enqueueReTask 将输入事件转换为任务并添加到队列
func (r *ReAct) enqueueReTask(event *ypb.AIInputEvent) error {
	// 创建基于aireact.Task的任务（初始状态为created）
	sanitizedInput := sanitizeForTaskId(event.FreeInput)
	shortId := ksuid.New().String()
	if len(shortId) > 8 {
		shortId = shortId[:8]
	}
	taskId := fmt.Sprintf("react-%s-%s", sanitizedInput, shortId)
	task := aicommon.NewStatefulTaskBase(
		taskId,
		event.FreeInput,
		r.config.GetContext(),
		r.Emitter)
	if r.config.DebugEvent {
		log.Infof("Task created: %s with input: %s", task.GetId(), event.FreeInput)
	}

	var attachedDatas []*aicommon.AttachedResource
	if len(event.AttachedResourceInfo) > 0 {
		for _, resource := range event.AttachedResourceInfo {
			attachedDatas = append(attachedDatas, aicommon.NewAttachedResource(resource.GetType(), resource.GetKey(), resource.GetValue()))
		}
	}
	task.SetAttachedDatas(attachedDatas)

	log.Infof("Task enqueue started processing: %s", task.GetId())
	// 任务不相关，进入排队状态
	task.SetFocusMode(event.GetFocusModeLoop())
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
