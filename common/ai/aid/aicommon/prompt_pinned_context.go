package aicommon

import (
	"encoding/json"
	"slices"
)

func (o TimelinePromptOptions) hidesPinnedDefinition(op *PromotableTimelineItem) bool {
	if op == nil {
		return false
	}
	if op.Kind == TimelinePromotedKindRecentTool {
		return slices.Contains(o.PinnedToolNames, op.Key)
	}
	if op.Kind != TimelinePromotedKindSkillContent || len(o.PinnedSkillNames) == 0 {
		return false
	}
	var payload skillContentPayload
	if json.Unmarshal([]byte(op.Payload), &payload) != nil {
		return false
	}
	return slices.Contains(o.PinnedSkillNames, payload.Name) &&
		(payload.Kind == skillContentKindMetadata || payload.Path == skillMDPathPlaceholder)
}

// Build a detached projection. Neither the journal nor a committed frozen
// prefix is rewritten when a focus is entered or left.
func (o TimelinePromptOptions) withoutPinnedDefinitions(state *TimelinePromotedState) *TimelinePromotedState {
	if state == nil || len(o.PinnedSkillNames)+len(o.PinnedToolNames) == 0 {
		return state
	}
	view := cloneTimelinePromotedState(state)
	for _, kinds := range view.Entries {
		for kind, entries := range kinds {
			for key, entry := range entries {
				if entry == nil {
					continue
				}
				hide := kind == TimelinePromotedKindRecentTool && slices.Contains(o.PinnedToolNames, key)
				if kind == TimelinePromotedKindSkillContent {
					name, _, partKind, path, _, _ := skillPromotedGroupKey(entry)
					hide = slices.Contains(o.PinnedSkillNames, name) && (partKind == skillContentKindMetadata || path == skillMDPathPlaceholder)
				}
				if hide {
					delete(entries, key)
				}
			}
		}
	}
	return view
}
