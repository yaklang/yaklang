package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// Called once before assembling the next loop request, never during an action
// or in a Timeline writer. Independent prompt fields are actual rendered values.
func (r *ReActLoop) compressTimelineBeforePrompt(userInput, frozenUserContext, todo, instruction string) error {
	provider, ok := r.config.(interface{ GetTimeline() *aicommon.Timeline })
	if !ok || provider.GetTimeline() == nil {
		return nil
	}
	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil {
		ctx = task.GetContext()
	}
	_, err := provider.GetTimeline().CompressBeforePrompt(aicommon.TimelineCompressionOptions{
		Context: ctx,
		RetainedContext: map[string]string{"user_query": userInput, "frozen_user_context": frozenUserContext,
			"todo": todo, "task_instruction": instruction},
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warnf("timeline compression before-prompt check failed; preserving complete history: %v", err)
	}
	return nil
}
