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
	tl := r.config.Timeline
	persister, enabled := r.config.MemoryTriage.(aimem.TimelineMemoryPersister)
	if !enabled || tl == nil || tl.IsBranchTimeline() {
		return
	}
	r.config.SyncSessionEvidenceTimeline()
	if !tl.NeedsMemoryFinalization() {
		return
	}
	// Only the local recovery checkpoint holds task completion. Waiting for
	// memory tails, backend initialization and embedding/index writes belongs
	// to the session, so the next queued task can use the live timeline now.
	if err := tl.RequestMemoryFinalization(); err != nil {
		log.Warnf("checkpoint user task memory failed: %v", err)
		return
	}
	ctx := r.config.GetContext()
	r.memoryFinalizationMu.Lock()
	defer r.memoryFinalizationMu.Unlock()
	if ctx.Err() != nil {
		return // The checkpoint will be replayed by the next runtime.
	}
	r.memoryFinalizationPending = func() error {
		return aimem.PersistPendingTimelineMemory(tl, persister, ctx)
	}
	if r.memoryFinalizationDone == nil {
		r.memoryFinalizationDone = make(chan struct{})
		go r.runMemoryFinalization()
	}
}

// One worker and at most one queued drain per runtime. A task ending during a
// save requests another pass, which snapshots the newest candidates; bursts do
// not create unbounded goroutines or retain completed task objects.
func (r *ReAct) runMemoryFinalization() {
	for {
		r.memoryFinalizationMu.Lock()
		pending := r.memoryFinalizationPending
		r.memoryFinalizationPending = nil
		if pending == nil || r.config.GetContext().Err() != nil {
			close(r.memoryFinalizationDone)
			r.memoryFinalizationDone = nil
			r.memoryFinalizationMu.Unlock()
			return
		}
		r.memoryFinalizationMu.Unlock()
		func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("user task memory persistence panic; source retained for recovery: %v", err)
				}
			}()
			if err := pending(); err != nil {
				log.Warnf("user task memory persistence failed; source retained for recovery: %v", err)
			}
		}()
	}
}

func (r *ReAct) waitMemoryFinalization() {
	r.memoryFinalizationMu.Lock()
	done := r.memoryFinalizationDone
	r.memoryFinalizationMu.Unlock()
	if done != nil {
		<-done
	}
}

// Artifact/result records must be appended before the async completion becomes
// observable. Review-only continuations still skip extraction here.
func (r *ReAct) finishAsyncUserTaskMemory(task aicommon.AIStatefulTask, err error, release func()) {
	defer r.collectTerminalTrajectory(task)
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
