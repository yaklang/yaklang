package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/log"
)

func (r *ReAct) beginUserTaskMemory(task aicommon.AIStatefulTask) func() {
	if task == nil || task.IsSubAgent() || (r.config.Timeline == nil || r.config.Timeline.IsBranchTimeline()) {
		return func() {}
	}
	if _, ok := r.memoryTriage.(aimem.TimelineMemoryPersister); !ok {
		return func() {}
	}
	if holder, ok := task.(interface{ DeferCompletion() func() }); ok {
		release := holder.DeferCompletion()
		return func() {
			_, suspended := r.memoryContinuations.Load(task.GetId())
			interrupted := false
			if loop, ok := task.GetReActLoop().(interface{ IsMaxIterationInterrupted() bool }); ok {
				interrupted = loop.IsMaxIterationInterrupted()
			}
			if !suspended && (task.IsUserCancelled() || r.config.GetContext().Err() != nil || task.GetStatus() == aicommon.AITaskState_Aborted || interrupted) {
				if err := r.config.Timeline.RequestMemoryFinalization(); err != nil {
					log.Warnf("save interrupted task memory source failed: %v", err)
				}
			}
			release()
		}
	}
	return func() {}
}

func (r *ReAct) completeUserTaskMemory(task aicommon.AIStatefulTask) {
	if task == nil || task.IsSubAgent() || task.IsUserCancelled() || r.config.GetContext().Err() != nil {
		return
	}
	if _, suspended := r.memoryContinuations.Load(task.GetId()); suspended {
		return
	}
	if loop, ok := task.GetReActLoop().(interface{ IsMaxIterationInterrupted() bool }); ok && loop.IsMaxIterationInterrupted() {
		return
	}
	if err := aimem.CompleteTimelineMemory(r.config, r.config.GetContext(), task.GetOriginUserInput()); err != nil {
		log.Warnf("user task memory finalization failed; source retained for recovery: %v", err)
	}
}

// Artifact/result records must be appended before the async completion becomes
// observable. Review-only continuations still skip extraction here.
func (r *ReAct) finishAsyncUserTaskMemory(task aicommon.AIStatefulTask, err error, release func()) {
	if task != nil {
		defer r.memoryContinuations.Delete(task.GetId())
	}
	defer release()
	if err == nil {
		r.completeUserTaskMemory(task)
	} else if _, enabled := r.memoryTriage.(aimem.TimelineMemoryPersister); enabled && task != nil {
		if _, waiting := r.memoryContinuations.Load(task.GetId()); !waiting {
			if saveErr := r.config.Timeline.RequestMemoryFinalization(); saveErr != nil {
				log.Warnf("save failed async task memory source failed: %v", saveErr)
			}
		}
	}
}
