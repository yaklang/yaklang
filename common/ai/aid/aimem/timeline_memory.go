package aimem

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// MemoryEntitySaver accepts already extracted entities; it has no AI extraction
// methods. MemoryStore and the existing asynchronous backend both implement it.
type MemoryEntitySaver interface {
	SaveMemoryEntities(...*aicommon.MemoryEntity) error
}

// TimelineMemoryPersister is separate from the legacy best-effort batch API.
// Implementations journal candidates and resume pending stages on empty input.
type TimelineMemoryPersister interface {
	PersistTimelineMemories(context.Context, []any) error
}

func (r *AIMemoryTriage) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	return r.memoryStore().PersistTimelineMemories(ctx, candidates)
}

// RegisterTimelineMemoryPersistence binds storage to the owning session only.
// Its asynchronous memory callback is shared by task forks, uses the session's
// existing backend, and never asks AI to extract or judge the candidates again.
func RegisterTimelineMemoryPersistence(timeline *aicommon.Timeline, saver MemoryEntitySaver, contexts ...context.Context) {
	if timeline == nil || saver == nil || !timeline.IsSessionTimeline() {
		return
	}
	persister, ok := saver.(TimelineMemoryPersister)
	if !ok {
		// Disabled/no-op and custom legacy savers do not opt into this channel.
		return
	}
	ctx := context.Background()
	if len(contexts) > 0 && contexts[0] != nil {
		ctx = contexts[0]
	}
	persist := func() {
		candidates, sources := timeline.GetMemoryPersistenceBatch()
		ctx, cancel := context.WithTimeout(ctx, TimelineMemoryFinalizationTimeout)
		defer cancel()
		for attempt := 0; attempt < 3; attempt++ {
			if ctx.Err() != nil {
				return // Durable pending receipts are replayed by the next runtime.
			}
			if err := persister.PersistTimelineMemories(ctx, candidates); err == nil {
				if err := timeline.AcknowledgeMemoryPersistence(sources); err != nil {
					log.Warnf("save memory persistence checkpoint failed: %v", err)
				}
				return
			} else {
				log.Warnf("timeline memory persistence attempt %d failed: %v", attempt+1, err)
			}
			if attempt == 2 {
				return
			}
			timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
	attached := timeline.RegisterSessionMemoryCallbackOnce("memory.persist", func(event aicommon.TimelineMemoryEvent) {
		if event.Err != nil {
			log.Warnf("timeline memory tail failed through item %d: %v", event.ThroughID, event.Err)
			return
		}
		persist()
	})
	if !attached {
		return
	}
	if err := timeline.PrepareMemoryRecovery(); err != nil {
		log.Warnf("prepare timeline memory recovery failed: %v", err)
	}
	// Reattach on restore and recover both the durable outbox and historical
	// candidates whose notification was lost before it could enter that outbox.
	go func() {
		persist()
		recoveryCtx, cancel := context.WithTimeout(ctx, TimelineMemoryFinalizationTimeout)
		defer cancel()
		if err := timeline.RecoverMemory(recoveryCtx); err != nil {
			log.Warnf("timeline memory recovery failed: %v", err)
			return
		}
		candidates, sources := timeline.GetMemoryPersistenceBatch()
		if err := persister.PersistTimelineMemories(recoveryCtx, candidates); err != nil {
			log.Warnf("timeline memory recovery persistence failed: %v", err)
			return
		}
		if err := timeline.AcknowledgeMemoryPersistence(sources); err != nil {
			log.Warnf("save memory persistence checkpoint failed: %v", err)
			return
		}
		if err := timeline.FinishMemoryFinalization(); err != nil {
			log.Warnf("save memory recovery checkpoint failed: %v", err)
		}
	}()
}
