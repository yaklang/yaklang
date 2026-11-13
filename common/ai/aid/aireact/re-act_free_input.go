package aireact

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) handleFreeValue(event *ypb.AIInputEvent) error {
	if event.GetUserInteraction() != "" {
		err := r.config.AcceptQuestionAnswer(event, func(text, taskID string, origin *aicommon.Config) (string, error) {
			if origin != nil {
				stamp := time.Now()
				// The ordinary input entry point journals into Timeline Open.
				// Only the existing freeze transaction promotes it to SemiDynamic1.
				history, err := r.config.AppendUserInputHistory(text, stamp)
				if err != nil {
					return "", err
				}
				if origin.GetSessionPromptState() != r.config.GetSessionPromptState() {
					if _, err := origin.AppendUserInputHistory(text, stamp); err != nil {
						return "", err
					}
				}
				if r.config.PersistentSessionId != "" && r.config.GetDB() != nil {
					if err := yakit.UpdateAIAgentRuntimeUserInput(r.config.GetDB(), r.config.GetRuntimeId(), history); err != nil {
						log.Warnf("persist UI reply history failed: %v", err)
					}
				}
				r.SaveTimeline()
				// Existing coordinator wakeup hooks consume this after admission.
				event.SyncType = aicommon.SYNC_TYPE_USER_INTERVENTION
				payload, _ := json.Marshal(map[string]string{"content": text})
				event.SyncJsonInput = string(payload)
				return "", nil
			}
			return r.enqueueReTaskWithID(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: text})
		})
		if err != nil {
			var receipt struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal([]byte(event.UserInteraction), &receipt)
			r.config.EmitSyncJSON("ui_receipt", "ui_receipt", map[string]any{"id": receipt.ID, "ok": false, "error": err.Error()}, event.SyncID)
		}
		return err
	}
	if r.pureInvokerMode {
		return utils.Errorf("use in prue invoker mode, cannot handle free input")
	}
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

func (r *ReAct) AddRuntimeTask(task aicommon.AIStatefulTask) {
	r.UpdateRuntimeTaskMutex.Lock()
	defer r.UpdateRuntimeTaskMutex.Unlock()
	r.RuntimeTasks = append(r.RuntimeTasks, task)
	if r.config.DebugEvent {
		log.Infof("Runtime task added: %s", task.GetId())
	}
}

func (r *ReAct) setCurrentTask(task aicommon.AIStatefulTask) {
	if task != nil {
		// Initialize the task before publishing it. Readers may start using the
		// task as soon as GetCurrentTask returns, so SetDB belongs on the writer's
		// private side of the ownership hand-off.
		task.SetDB(r.config.GetDB())
	}

	r.currentTaskMu.Lock()
	r.lastTask = r.currentTask
	r.currentTask = task
	r.currentTaskMu.Unlock()
	if r.config.DebugEvent {
		if task != nil {
			log.Infof("Current task set to: %s", task.GetId())
		}
	}
}

func (r *ReAct) IsProcessingReAct() bool {
	return r.GetCurrentTask() != nil
}

func (r *ReAct) GetLastTask() aicommon.AIStatefulTask {
	r.currentTaskMu.RLock()
	lastTask := r.lastTask
	r.currentTaskMu.RUnlock()
	if lastTask == nil {
		return nil
	}
	if r.config.DebugEvent {
		log.Infof("Last task retrieved: %s", lastTask.GetId())
	}
	return lastTask
}

func (r *ReAct) GetCurrentTask() aicommon.AIStatefulTask {
	r.currentTaskMu.RLock()
	currentTask := r.currentTask
	r.currentTaskMu.RUnlock()
	if currentTask == nil {
		return nil
	}
	if r.config.DebugEvent {
		log.Infof("Current task retrieved: %s", currentTask.GetId())
	}
	return currentTask
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
	loop := currentTask.GetReActLoop()
	if loop == nil {
		return nil
	}
	currentLoop, ok := loop.(*reactloops.ReActLoop)
	if !ok || currentLoop == nil {
		return nil
	}
	return currentLoop
}

func (r *ReAct) DumpCurrentEnhanceData() string {
	if r.config.EnhanceKnowledgeManager == nil {
		return ""
	}
	data := r.config.EnhanceKnowledgeManager.DumpTaskAboutKnowledge(r.GetCurrentTaskId())
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
	_, err := r.enqueueReTaskWithID(event)
	return err
}

func (r *ReAct) enqueueReTaskWithID(event *ypb.AIInputEvent) (string, error) {
	resources, control := normalizeInputAttachments(event)
	task := r.buildReTaskFromEvent(event, resources, control)
	if task == nil {
		return "", fmt.Errorf("failed to build task from event")
	}
	// 任务不相关，进入排队状态
	task.SetFocusMode(event.GetFocusModeLoop())
	task.SetStatus(aicommon.AITaskState_Queueing)
	taskInput := r.buildSessionTaskInput(task, resources)
	log.Infof("Task enqueue started processing: %s", task.GetId())
	err := r.taskQueue.Append(task)
	if err != nil {
		log.Errorf("Failed to add task to queue: %v", err)
		return "", fmt.Errorf("failed to enqueue task: %v", err)
	}
	r.saveAISessionTaskInput(taskInput)
	if r.config.DebugEvent {
		log.Infof("Task enqueued: %s with input: %s", task.GetId(), event.FreeInput)
	}
	return task.GetId(), nil
}

type scheduledTaskInputControl struct {
	uuid        string
	name        string
	scheduledAt string
	trigger     string
}

type taskInputControl struct {
	userInputUUID string
	inputSource   string
	schedule      *scheduledTaskInputControl
}

func normalizeInputAttachments(event *ypb.AIInputEvent) ([]*aicommon.AttachedResource, *taskInputControl) {
	var resources = make([]*aicommon.AttachedResource, 0)
	control := &taskInputControl{schedule: &scheduledTaskInputControl{}}
	if event == nil {
		return resources, control
	}

	seen := make(map[string]struct{})
	appendResource := func(resource *aicommon.AttachedResource) {
		if resource == nil {
			return
		}
		value := strings.TrimSpace(resource.Value)
		if resource.HasType(aicommon.AttachedResourceTypeFile) && resource.HasKey(aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT) {
			value = resource.Value
		}
		key := resource.NormalizedType() + "\x00" + strings.TrimSpace(resource.Key) + "\x00" + value
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		resources = append(resources, resource)
	}

	for _, path := range event.GetAttachedFilePath() {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		appendResource(aicommon.NewAttachedResource(
			aicommon.CONTEXT_PROVIDER_TYPE_FILE,
			aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
			path,
		))
	}

	for _, resource := range event.GetAttachedResourceInfo() {
		if resource == nil {
			continue
		}
		switch resource.GetType() {
		case aicommon.USER_FREE_INPUT_UUID:
			control.userInputUUID = resource.GetValue()
			continue
		case aicommon.USER_INPUT_SOURCE:
			if resource.GetKey() == aicommon.USER_INPUT_SOURCE_KEY {
				control.inputSource = resource.GetValue()
				continue
			}
		case aicommon.USER_INPUT_SCHEDULE_CONTEXT:
			switch resource.GetKey() {
			case aicommon.USER_INPUT_SCHEDULE_UUID:
				control.schedule.uuid = resource.GetValue()
			case aicommon.USER_INPUT_SCHEDULE_NAME:
				control.schedule.name = resource.GetValue()
			case aicommon.USER_INPUT_SCHEDULED_AT:
				control.schedule.scheduledAt = resource.GetValue()
			case aicommon.USER_INPUT_SCHEDULE_TRIGGER:
				control.schedule.trigger = resource.GetValue()
			}
			continue
		}
		appendResource(aicommon.NewAttachedResource(resource.GetType(), resource.GetKey(), resource.GetValue()))
	}
	return resources, control
}

func (r *ReAct) buildReTaskFromEvent(event *ypb.AIInputEvent, resources []*aicommon.AttachedResource, control *taskInputControl) aicommon.AIStatefulTask {
	if event == nil {
		return nil
	}
	if control == nil {
		control = &taskInputControl{schedule: &scheduledTaskInputControl{}}
	}
	if control.schedule == nil {
		control.schedule = &scheduledTaskInputControl{}
	}
	freeInput := event.FreeInput
	// 创建基于aireact.Task的任务（初始状态为created）
	sanitizedInput := sanitizeForTaskId(freeInput)
	shortId := ksuid.New().String()
	if len(shortId) > 8 {
		shortId = shortId[:8]
	}
	taskId := fmt.Sprintf("react-%s-%s", sanitizedInput, shortId)
	task := aicommon.NewStatefulTaskBase(
		taskId,
		freeInput,
		r.config.GetContext(),
		r.Emitter)
	if r.config.DebugEvent {
		log.Infof("Task created: %s with input: %s", task.GetId(), event.FreeInput)
	}

	attachedDatas := make([]*aicommon.AttachedResource, 0, len(resources))
	task.SetUserInputUUID(control.userInputUUID)
	task.SetInputSource(control.inputSource)
	task.SetScheduleUUID(control.schedule.uuid)
	task.SetScheduleName(control.schedule.name)
	task.SetScheduledAt(control.schedule.scheduledAt)
	task.SetScheduleTrigger(control.schedule.trigger)
	r.bindBrowserAttachmentsToTask(task, resources)
	for _, attached := range resources {
		if attached.HasType(aicommon.AttachedResourceTypeBrowser) {
			continue
		}
		attachedDatas = append(attachedDatas, attached)
	}
	task.SetAttachedDatas(attachedDatas)
	return task
}
