package aireact

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func isAISessionTaskControlResource(resource *ypb.AttachedResourceInfo) bool {
	if resource == nil {
		return true
	}
	switch resource.GetType() {
	case aicommon.USER_FREE_INPUT_UUID:
		return true
	case aicommon.USER_INPUT_SOURCE:
		return resource.GetKey() == aicommon.USER_INPUT_SOURCE_KEY
	case aicommon.USER_INPUT_SCHEDULE_CONTEXT:
		return true
	default:
		return false
	}
}

func buildAISessionTaskInputParams(sessionID string, task aicommon.AIStatefulTask, event *ypb.AIInputEvent) *yakit.SaveAISessionTaskInputParams {
	if task == nil || event == nil {
		return nil
	}
	resources := make([]yakit.AISessionTaskResource, 0, len(event.GetAttachedFilePath())+len(event.GetAttachedResourceInfo()))
	for _, path := range event.GetAttachedFilePath() {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		resources = append(resources, yakit.AISessionTaskResource{
			Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
			Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
			Value: path,
		})
	}
	for _, resource := range event.GetAttachedResourceInfo() {
		if isAISessionTaskControlResource(resource) {
			continue
		}
		resources = append(resources, yakit.AISessionTaskResource{
			Type:  resource.GetType(),
			Key:   resource.GetKey(),
			Value: resource.GetValue(),
		})
	}

	return &yakit.SaveAISessionTaskInputParams{
		SessionID:       sessionID,
		TaskID:          task.GetId(),
		TaskUUID:        task.GetUUID(),
		UserInputUUID:   task.GetUserInputUUID(),
		UserInput:       task.GetUserInput(),
		InputSource:     task.GetInputSource(),
		FocusModeLoop:   task.GetFocusMode(),
		ScheduleUUID:    task.GetScheduleUUID(),
		ScheduleName:    task.GetScheduleName(),
		ScheduledAt:     task.GetScheduledAt(),
		ScheduleTrigger: task.GetScheduleTrigger(),
		Resources:       resources,
	}
}

func (r *ReAct) buildSessionTaskInput(task aicommon.AIStatefulTask, event *ypb.AIInputEvent) *yakit.SaveAISessionTaskInputParams {
	if r == nil || r.config == nil || task == nil {
		return nil
	}
	sessionID := strings.TrimSpace(r.config.PersistentSessionId)
	if sessionID == "" {
		return nil
	}
	return buildAISessionTaskInputParams(sessionID, task, event)
}

func (r *ReAct) saveAISessionTaskInput(params *yakit.SaveAISessionTaskInputParams) {
	if r == nil || r.config == nil || params == nil {
		return
	}
	if err := yakit.SaveAISessionTaskInput(r.config.GetDB(), params); err != nil {
		log.Warnf("save AI session task input failed for session %s task %s: %v", params.SessionID, params.TaskID, err)
	}
}
