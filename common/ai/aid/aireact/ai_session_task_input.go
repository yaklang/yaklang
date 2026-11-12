package aireact

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func buildAISessionTaskInputParams(sessionID string, task aicommon.AIStatefulTask, resources []*aicommon.AttachedResource) *yakit.SaveAISessionTaskInputParams {
	if task == nil {
		return nil
	}
	sessionResources := make([]yakit.AISessionTaskResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		sessionResources = append(sessionResources, yakit.AISessionTaskResource{
			Type:  resource.Type,
			Key:   resource.Key,
			Value: resource.Value,
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
		Resources:       sessionResources,
	}
}

func (r *ReAct) buildSessionTaskInput(task aicommon.AIStatefulTask, resources []*aicommon.AttachedResource) *yakit.SaveAISessionTaskInputParams {
	if r == nil || r.config == nil || task == nil {
		return nil
	}
	sessionID := strings.TrimSpace(r.config.PersistentSessionId)
	if sessionID == "" {
		return nil
	}
	return buildAISessionTaskInputParams(sessionID, task, resources)
}

func (r *ReAct) saveAISessionTaskInput(params *yakit.SaveAISessionTaskInputParams) {
	if r == nil || r.config == nil || params == nil {
		return
	}
	if err := yakit.SaveAISessionTaskInput(r.config.GetDB(), params); err != nil {
		log.Warnf("save AI session task input failed for session %s task %s: %v", params.SessionID, params.TaskID, err)
	}
}
