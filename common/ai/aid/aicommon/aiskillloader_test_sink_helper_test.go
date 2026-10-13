package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
)

// bindTestTimeline wires a manager to a real Timeline using the production
// adapter, the same path the ReAct loop uses. Tests then assert on the frozen
// skill block produced by Timeline, not on manager-side rendering.
func bindTestTimeline(mgr *aiskillloader.SkillsContextManager, cfg *Config) {
	if mgr == nil || cfg == nil || cfg.GetTimeline() == nil {
		return
	}
	if sink := NewSkillContentTimelineAdapter(cfg.GetTimeline(), cfg.AcquireId); sink != nil {
		mgr.BindTimelineSink(sink)
	}
}
