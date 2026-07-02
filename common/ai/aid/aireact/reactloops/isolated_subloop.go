package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// RunSubLoopIsolated executes a registered sub-loop and rolls back shared timeline
// entries created during the run so parent loops are not polluted by grep/read noise.
func RunSubLoopIsolated(
	invoker aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	loopName string,
	configure func(subLoop *ReActLoop),
	opts ...ReActLoopOption,
) (*ReActLoop, error) {
	if invoker == nil {
		return nil, utils.Error("invoker is nil")
	}

	var timeline *aicommon.Timeline
	var checkpoint int64
	if cfg := invoker.GetConfig(); cfg != nil {
		if c, ok := cfg.(*aicommon.Config); ok && c.Timeline != nil {
			timeline = c.Timeline
			checkpoint = timeline.GetMaxID()
			defer func() {
				removed := countTimelineIDsAfter(timeline, checkpoint)
				timeline.TruncateAfter(checkpoint)
				if removed > 0 {
					log.Infof("[ReActLoop] sub-loop %s timeline rollback: removed %d entries", loopName, removed)
				}
			}()
		}
	}

	factory, ok := GetLoopFactory(loopName)
	if !ok || factory == nil {
		return nil, utils.Errorf("reactloop[%s] not found", loopName)
	}

	subLoop, err := factory(invoker, opts...)
	if err != nil {
		return nil, utils.Wrap(err, "create sub-loop")
	}
	if configure != nil {
		configure(subLoop)
	}

	if task == nil {
		if cfg, ok := invoker.GetConfig().(*aicommon.Config); ok && cfg.DefaultTask != nil {
			task = cfg.DefaultTask
		}
	}
	if task == nil {
		return subLoop, utils.Error("parent task is required for isolated sub-loop execution")
	}

	if err := subLoop.ExecuteWithExistedTask(task); err != nil {
		return subLoop, utils.Wrap(err, "execute sub-loop")
	}
	return subLoop, nil
}

func countTimelineIDsAfter(timeline *aicommon.Timeline, checkpoint int64) int {
	if timeline == nil {
		return 0
	}
	count := 0
	for _, id := range timeline.GetTimelineItemIDs() {
		if id > checkpoint {
			count++
		}
	}
	return count
}
