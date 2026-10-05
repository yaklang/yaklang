package aimem

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const TimelineMemoryFinalizationTimeout = 120 * time.Second

// CompleteTimelineMemory is the full user-task boundary, not a loop/worker
// boundary. A cancelled/disconnected runtime only journals pending work; a new
// runtime may replay it with a fresh bounded context. Persistence failure leaves
// that intent and the original/retry source intact.
func CompleteTimelineMemory(cfg *aicommon.Config, ctx context.Context, userInput string) error {
	if cfg == nil || cfg.Timeline == nil || cfg.Timeline.IsBranchTimeline() {
		return nil
	}
	persister, ok := cfg.MemoryTriage.(TimelineMemoryPersister)
	if !ok {
		return nil
	}
	cfg.SyncSessionEvidenceTimeline()
	return finalizeTimelineMemory(cfg.Timeline, persister, ctx, userInput)
}

func finalizeTimelineMemory(tl *aicommon.Timeline, persister TimelineMemoryPersister, ctx context.Context, userInput string) error {
	if !tl.NeedsMemoryFinalization() {
		return nil
	}
	if err := tl.RequestMemoryFinalization(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, TimelineMemoryFinalizationTimeout)
	defer cancel()
	if err := tl.FinalizeMemory(ctx, aicommon.TimelineCompressionOptions{
		RetainedContext: map[string]string{"user_query": userInput},
	}); err != nil {
		return err
	}
	candidates, sources := tl.GetMemoryPersistenceBatch()
	if err := persister.PersistTimelineMemories(ctx, candidates); err != nil {
		return err
	}
	if err := tl.AcknowledgeMemoryPersistence(sources); err != nil {
		return err
	}
	return tl.FinishMemoryFinalization()
}
