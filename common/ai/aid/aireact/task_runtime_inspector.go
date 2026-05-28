package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

var _ aid.TaskRuntimeReportProvider = (*ReAct)(nil)

// CollectTaskRuntimeReport builds a runtime snapshot for async / executing tasks.
func (r *ReAct) CollectTaskRuntimeReport() *aid.TaskRuntimeReport {
	if r == nil {
		return aid.BuildTaskRuntimeReport(nil)
	}
	return aid.BuildTaskRuntimeReport(r)
}

func (r *ReAct) GetReActID() string {
	if r == nil || r.config == nil {
		return ""
	}
	return r.config.Id
}

func (r *ReAct) GetRuntimeTasks() []aicommon.AIStatefulTask {
	if r == nil {
		return nil
	}
	r.UpdateRuntimeTaskMutex.Lock()
	defer r.UpdateRuntimeTaskMutex.Unlock()
	return append([]aicommon.AIStatefulTask(nil), r.RuntimeTasks...)
}

func (r *ReAct) GetQueueingTasks() []aicommon.AIStatefulTask {
	if r == nil || r.taskQueue == nil {
		return nil
	}
	return r.taskQueue.GetQueueingTasks()
}

func (r *ReAct) GetSessionTimeline() *aicommon.Timeline {
	if r == nil || r.config == nil {
		return nil
	}
	return r.config.Timeline
}
