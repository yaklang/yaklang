package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func (t *AiTask) CurrentTimeline() *aicommon.Timeline {
	if t == nil {
		return nil
	}
	if t.timelineFork != nil && t.timelineFork.Branch != nil {
		return t.timelineFork.Branch
	}
	if t.Coordinator != nil && t.Coordinator.Timeline != nil {
		return t.Coordinator.Timeline
	}
	if t.Coordinator != nil && t.Coordinator.ContextProvider != nil {
		return t.Coordinator.ContextProvider.GetTimelineInstance()
	}
	return nil
}

func (t *AiTask) withTimelineFork(f *aicommon.TimelineFork) func() {
	prev := t.timelineFork
	t.timelineFork = f
	return func() {
		t.timelineFork = prev
	}
}

func (t *AiTask) CallAfterReview(seq int64, reviewQuestion string, userInput aitool.InvokeParams) {
	tl := t.CurrentTimeline()
	if tl == nil {
		return
	}
	tl.PushUserInteraction(aicommon.UserInteractionStage_Review, seq, reviewQuestion, string(utils.Jsonify(userInput)))
}
