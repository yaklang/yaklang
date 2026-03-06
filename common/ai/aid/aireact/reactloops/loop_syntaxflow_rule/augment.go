package loop_syntaxflow_rule

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	loopinfra.RegisterFocusModeUserInputAugmenter(schema.AI_REACT_LOOP_NAME_WRITE_SYNTAXFLOW, augmentUserInputForSyntaxFlow)
}

// augmentUserInputForSyntaxFlow augments user input with full content from timeline
// when PE/UI created a subtask with only the goal text.
func augmentUserInputForSyntaxFlow(cfg aicommon.AICallerConfigIf, userInput string) string {
	if augmented := tryAugmentUserInputFromTimeline(cfg, userInput); augmented != "" {
		return augmented
	}
	return userInput
}

// tryAugmentUserInputFromTimeline extracts "current task user input" entries from
// timeline. When PE or UI creates a subtask with only the goal, the root task's
// full user input (including code) is in the timeline.
func tryAugmentUserInputFromTimeline(cfg aicommon.AICallerConfigIf, currentInput string) string {
	config, ok := cfg.(*aicommon.Config)
	if !ok || config == nil || config.Timeline == nil {
		return ""
	}
	outputs := config.Timeline.GetTimelineOutput()
	if outputs == nil {
		return ""
	}
	marker := "[current task user input]"
	var best string
	for _, out := range outputs {
		if out == nil || out.Type != "text" {
			continue
		}
		raw := out.Content
		if !strings.Contains(raw, marker) {
			continue
		}
		idx := strings.Index(raw, marker)
		after := raw[idx:]
		colonIdx := strings.Index(after, ":\n")
		if colonIdx == -1 {
			colonIdx = strings.Index(after, ":")
		}
		if colonIdx < 0 {
			continue
		}
		content := strings.TrimSpace(after[colonIdx+2:])
		content = strings.ReplaceAll(content, "\n  ", "\n")
		content = strings.TrimSpace(content)
		if len(content) > len(best) && len(content) > len(currentInput) {
			best = content
		}
	}
	return best
}
