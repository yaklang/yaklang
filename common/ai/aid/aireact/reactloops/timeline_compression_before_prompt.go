package reactloops

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

var contextOptimizationStatusIndex atomic.Uint64

var contextOptimizationStatusNames = []actionStatusName{
	{"正在优化上下文…", "Optimizing the context…"},
	{"正在整理历史记录，保留关键线索…", "Organizing the history and keeping key findings…"},
	{"正在精简上下文，保留重要信息…", "Compacting the context and keeping important information…"},
	{"正在归纳前面的进展…", "Summarizing progress so far…"},
	{"正在梳理上下文，准备继续…", "Organizing the context before continuing…"},
}

// Called once before assembling the next loop request, never during an action
// or in a Timeline writer. Independent prompt fields are actual rendered values.
func (r *ReActLoop) compressTimelineBeforePrompt(userInput, frozenUserContext, todo string) error {
	if config, ok := r.config.(*aicommon.Config); ok {
		config.SyncSessionEvidenceTimeline()
	}
	provider, ok := r.config.(interface{ GetTimeline() *aicommon.Timeline })
	if !ok || provider.GetTimeline() == nil {
		return nil
	}
	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil {
		ctx = task.GetContext()
	}
	var optimizationLabel *actionStatusName
	var lastStage aicommon.TimelineCompressionStage
	defer func() {
		if lastStage != "" && ctx.Err() == nil {
			r.UserStatus("正在准备下一轮请求…", "Preparing the next request…",
				aicommon.WithStatusCode("response.preparing"))
		}
	}()
	_, err := provider.GetTimeline().CompressBeforePrompt(aicommon.TimelineCompressionOptions{
		Context: ctx,
		OnProgress: func(stage aicommon.TimelineCompressionStage) {
			if stage == lastStage || ctx.Err() != nil {
				return
			}
			lastStage = stage
			label := actionStatusName{"正在等待已有的上下文优化完成…", "Waiting for the ongoing context optimization…"}
			code := "context.optimization.waiting"
			switch stage {
			case aicommon.TimelineCompressionOptimizing:
				if optimizationLabel == nil {
					index := (contextOptimizationStatusIndex.Add(1) - 1) % uint64(len(contextOptimizationStatusNames))
					optimizationLabel = &contextOptimizationStatusNames[index]
				}
				label, code = *optimizationLabel, "context.optimizing"
			case aicommon.TimelineCompressionMemory:
				label = actionStatusName{"正在等待前一轮记忆整理完成…", "Waiting for the previous memory extraction…"}
				code = "context.optimization.memory"
			}
			r.UserStatus(label.zh, label.en, aicommon.WithStatusCode(code))
		},
		RetainedContext: map[string]string{"user_query": userInput, "frozen_user_context": frozenUserContext,
			"todo": todo},
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warnf("timeline compression before-prompt check failed; preserving complete history: %v", err)
	}
	return nil
}
