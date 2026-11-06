package aireact

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type aiSessionTaskResourceIdentity struct {
	typ   string
	key   string
	value string
}

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

func aiSessionTaskResourceScope(typ, key string) string {
	if typ == aicommon.AttachedResourceTypeBrowser ||
		(typ == aicommon.CONTEXT_PROVIDER_TYPE_FILE && key == aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH) {
		return yakit.AISessionTaskResourceScopeSession
	}
	return yakit.AISessionTaskResourceScopeTurn
}

func appendUniqueAISessionTaskResource(
	resources []yakit.AISessionTaskResource,
	seen map[aiSessionTaskResourceIdentity]struct{},
	resource yakit.AISessionTaskResource,
) []yakit.AISessionTaskResource {
	resource.Type = strings.TrimSpace(resource.Type)
	resource.Key = strings.TrimSpace(resource.Key)
	if resource.Type == "" && resource.Key == "" && resource.Value == "" {
		return resources
	}
	identity := aiSessionTaskResourceIdentity{
		typ: resource.Type, key: resource.Key, value: resource.Value,
	}
	if _, exists := seen[identity]; exists {
		return resources
	}
	seen[identity] = struct{}{}
	return append(resources, resource)
}

func buildAISessionTaskInputParams(sessionID string, task aicommon.AIStatefulTask, event *ypb.AIInputEvent) *yakit.SaveAISessionTaskInputParams {
	if task == nil || event == nil {
		return nil
	}
	explicit := make([]yakit.AISessionTaskResource, 0)
	explicitSeen := make(map[aiSessionTaskResourceIdentity]struct{})
	for _, path := range event.GetAttachedFilePath() {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		explicit = appendUniqueAISessionTaskResource(explicit, explicitSeen, yakit.AISessionTaskResource{
			Type:   aicommon.CONTEXT_PROVIDER_TYPE_FILE,
			Key:    aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
			Value:  path,
			Scope:  yakit.AISessionTaskResourceScopeSession,
			Source: yakit.AISessionTaskResourceSourceAttachedFilePath,
		})
	}
	for _, resource := range event.GetAttachedResourceInfo() {
		if isAISessionTaskControlResource(resource) {
			continue
		}
		typ := strings.TrimSpace(resource.GetType())
		key := strings.TrimSpace(resource.GetKey())
		explicit = appendUniqueAISessionTaskResource(explicit, explicitSeen, yakit.AISessionTaskResource{
			Type:   typ,
			Key:    key,
			Value:  resource.GetValue(),
			Scope:  aiSessionTaskResourceScope(typ, key),
			Source: yakit.AISessionTaskResourceSourceAttachedResourceInfo,
		})
	}

	effective := make([]yakit.AISessionTaskResource, 0, len(explicit)+len(task.GetAttachedDatas()))
	effectiveSeen := make(map[aiSessionTaskResourceIdentity]struct{})
	explicitByTypeKeyValue := make(map[aiSessionTaskResourceIdentity]bool, len(explicit))
	for _, resource := range explicit {
		explicitByTypeKeyValue[aiSessionTaskResourceIdentity{
			typ: resource.Type, key: resource.Key, value: resource.Value,
		}] = true
	}
	for _, resource := range task.GetAttachedDatas() {
		if resource == nil {
			continue
		}
		identity := aiSessionTaskResourceIdentity{
			typ: strings.TrimSpace(resource.Type), key: strings.TrimSpace(resource.Key), value: resource.Value,
		}
		source := yakit.AISessionTaskResourceSourceInherited
		if explicitByTypeKeyValue[identity] {
			for _, explicitResource := range explicit {
				if explicitResource.Type == identity.typ &&
					explicitResource.Key == identity.key &&
					explicitResource.Value == identity.value {
					source = explicitResource.Source
					break
				}
			}
		}
		effective = appendUniqueAISessionTaskResource(effective, effectiveSeen, yakit.AISessionTaskResource{
			Type:   identity.typ,
			Key:    identity.key,
			Value:  identity.value,
			Scope:  aiSessionTaskResourceScope(identity.typ, identity.key),
			Source: source,
		})
	}
	for _, resource := range explicit {
		effective = appendUniqueAISessionTaskResource(effective, effectiveSeen, resource)
	}

	filePaths := append([]string(nil), event.GetAttachedFilePath()...)
	return &yakit.SaveAISessionTaskInputParams{
		SessionID:          sessionID,
		TaskID:             task.GetId(),
		TaskUUID:           task.GetUUID(),
		UserInputUUID:      task.GetUserInputUUID(),
		UserInput:          task.GetUserInput(),
		InputSource:        task.GetInputSource(),
		FocusModeLoop:      task.GetFocusMode(),
		ScheduleUUID:       task.GetScheduleUUID(),
		ScheduleName:       task.GetScheduleName(),
		ScheduledAt:        task.GetScheduledAt(),
		ScheduleTrigger:    task.GetScheduleTrigger(),
		AttachedFilePaths:  filePaths,
		ExplicitResources:  explicit,
		EffectiveResources: effective,
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
