package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func TestTimelineMemoryFinalizationExactOnlyAndUnchanged(t *testing.T) {
	for _, kind := range []string{"user", "evidence", "empty-memory"} {
		t.Run(kind, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			if kind == "evidence" {
				importFreezeItem(tl, 1, time.Unix(1, 0), &PromotableTimelineItem{ID: 1,
					Kind: TimelinePromotedKindEvidence, TargetSection: TimelinePromotedTargetSemiDynamic1,
					Key: "e1", Operation: TimelinePromotedOperationUpsert, Payload: `{"id":"e1","content":"EXACT_EVIDENCE"}`})
			} else {
				tl.EnsureTaskUserInput("short", "EXACT_USER: use concise Chinese reports", func() int64 { return 1 })
			}
			calls := 0
			bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
				calls++
				require.Contains(t, req.GetPrompt(), "EXACT_")
				if kind == "empty-memory" {
					return compressionMockSummary("short task completed"), nil
				}
				raw, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
				return string(raw), err
			})
			// Promotion/freezing itself must not invoke the model.
			_, err := tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.Zero(t, calls)
			tl.RequestMemoryFinalization()
			require.NoError(t, tl.FinalizeMemory(context.Background(), compressionTestOptions()))
			tl.FinishMemoryFinalization()
			require.False(t, tl.HasPendingMemoryFinalization())
			_, sources := tl.GetMemoryPersistenceBatch()
			require.NoError(t, tl.AcknowledgeMemoryPersistence(sources))
			require.False(t, tl.NeedsMemoryFinalization())
			require.Equal(t, 1, calls)
			tl.PushText(2, "[MODEL_THINKING]:\nprivate reasoning")
			tl.PushText(3, "[ITERATION]:\niteration bookkeeping")
			for n := 0; n < 3; n++ {
				tl.RequestMemoryFinalization()
				require.NoError(t, tl.FinalizeMemory(context.Background(), compressionTestOptions()))
				tl.FinishMemoryFinalization()
			}
			require.Equal(t, 1, calls, "unchanged and empty memory must not repeat extraction")
			prompt := RenderTimelineFrozenOpen(tl)
			for _, internal := range []string{"memory_retries", "memory_processed_sources", "memory_covered_state", "memory_finalize_pending"} {
				require.NotContains(t, prompt.Frozen+prompt.Open, internal)
			}
		})
	}
}

func TestTimelineMemoryFinalizationWaitsTailAndPreservesOnTimeout(t *testing.T) {
	registerTimelineTestLiteForge(t)
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ORIGINAL_BUSINESS_SOURCE")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	cfg := NewTestConfig(ctx, WithDisableCreateDBRuntime(true), WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			r := NewUnboundAIResponse()
			r.EmitOutputStream(reader)
			r.Close()
			return r, nil
		}))
	tl.SoftBindConfig(cfg, nil)
	done := make(chan error, 1)
	short, stop := context.WithTimeout(ctx, 150*time.Millisecond)
	defer stop()
	go func() { done <- tl.FinalizeMemory(short, compressionTestOptions()) }()
	_, err := io.WriteString(writer, `{"@action":"timeline-summary","summary":"usable summary","ratain_timeline_item_range":"","memory_entities":`)
	require.NoError(t, err)
	require.ErrorIs(t, <-done, context.DeadlineExceeded)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "ORIGINAL_BUSINESS_SOURCE")
	require.Empty(t, tl.GetSessionMemoryCandidates())
}

func TestTimelineMemoryFailedThresholdTailRestoresOriginalSource(t *testing.T) {
	registerTimelineTestLiteForge(t)
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "REPLAY_ORIGINAL_SOURCE")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close(); _ = reader.Close() })
	cfg := NewTestConfig(ctx, WithDisableCreateDBRuntime(true), WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, req *AIRequest) (*AIResponse, error) {
			r := NewUnboundAIResponse()
			r.EmitOutputStream(reader)
			r.Close()
			return r, nil
		}))
	tl.SoftBindConfig(cfg, nil)
	done := make(chan error, 1)
	go func() { _, err := tl.CompressOnce(compressionTestOptions()); done <- err }()
	_, err := io.WriteString(writer, `{"@action":"timeline-summary","summary":"early usable summary","ratain_timeline_item_range":"","memory_entities":`)
	require.NoError(t, err)
	require.NoError(t, <-done, "threshold summary must still publish early")
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Frozen, "REPLAY_ORIGINAL_SOURCE")
	require.NoError(t, writer.CloseWithError(io.ErrUnexpectedEOF))
	require.NoError(t, tl.sessionMemory.wait(ctx))
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Contains(t, raw, "REPLAY_ORIGINAL_SOURCE", "retired source needs a durable replay record")
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	calls := 0
	bindCompressionMock(t, restored, func(req *AIRequest) (string, error) {
		calls++
		require.Contains(t, req.GetPrompt(), "REPLAY_ORIGINAL_SOURCE")
		payload, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
		return string(payload), err
	})
	require.NoError(t, restored.FinalizeMemory(ctx, compressionTestOptions()))
	require.NoError(t, restored.FinalizeMemory(ctx, compressionTestOptions()))
	require.Equal(t, 1, calls)
	require.Len(t, restored.GetSessionMemoryCandidates(), 1)
	candidates, sources := restored.GetMemoryPersistenceBatch()
	require.Len(t, candidates, 1)
	require.Contains(t, mustMarshalMemoryTimeline(t, restored), "REPLAY_ORIGINAL_SOURCE", "save failure must retain the original source")
	restored.AcknowledgeMemoryPersistence(sources)
	require.Empty(t, restored.sessionMemory.retrySnapshot())
}

func TestTimelineMemoryRecoveryDoesNotExtractNewActiveInput(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "CANCELLED_SOURCE")
	tl.RequestMemoryFinalization()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, tl.FinalizeMemory(cancelled, compressionTestOptions()), context.Canceled)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.NoError(t, restored.PrepareMemoryRecovery())
	restored.PushText(2, "NEW_ACTIVE_TASK_MUST_NOT_FINALIZE")
	calls := 0
	bindCompressionMock(t, restored, func(req *AIRequest) (string, error) {
		calls++
		require.Contains(t, req.GetPrompt(), "CANCELLED_SOURCE")
		require.NotContains(t, req.GetPrompt(), "NEW_ACTIVE_TASK_MUST_NOT_FINALIZE")
		return compressionMockSummary("recovered memory source"), nil
	})
	require.NoError(t, restored.RecoverMemory(context.Background()))
	restored.FinishMemoryFinalization()
	require.True(t, restored.HasPendingMemoryFinalization(), "new input remains uncovered")
	require.NoError(t, restored.RecoverMemory(context.Background()))
	require.Equal(t, 1, calls)
	require.Contains(t, RenderTimelineFrozenOpen(restored).Open, "NEW_ACTIVE_TASK_MUST_NOT_FINALIZE")
}

func TestTimelineMemoryConcurrentAppendRemainsUncovered(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "first source")
	calls := 0
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		calls++
		if calls == 1 {
			tl.PushText(2, "NEW_DURING_COMPRESSION")
		} else {
			require.Contains(t, req.GetPrompt(), "NEW_DURING_COMPRESSION")
		}
		return compressionMockSummary("completed old source"), nil
	})
	tl.RequestMemoryFinalization()
	require.NoError(t, tl.FinalizeMemory(context.Background(), compressionTestOptions()))
	tl.FinishMemoryFinalization()
	require.True(t, tl.HasPendingMemoryFinalization())
	require.NoError(t, tl.FinalizeMemory(context.Background(), compressionTestOptions()))
	tl.FinishMemoryFinalization()
	require.False(t, tl.HasPendingMemoryFinalization())
	require.Equal(t, 2, calls)
}

func TestTimelineMemoryPrivateBranchPartialFailureAndDuplicateRelease(t *testing.T) {
	parent := NewTimeline(nil, nil)
	first, err := parent.ForkForTask("one", "private one", nil, nil)
	require.NoError(t, err)
	second, err := parent.ForkForTask("two", "private two", nil, nil)
	require.NoError(t, err)
	first.Branch.PushText(1, "PRIVATE_ONE")
	second.Branch.PushText(2, "PRIVATE_TWO")
	require.NoError(t, parent.ArchiveMemoryBranch(first.Branch, first.BaseMaxID))
	require.NoError(t, parent.ArchiveMemoryBranch(second.Branch, second.BaseMaxID))
	require.Empty(t, RenderTimelineFrozenOpen(parent).Open, "private history must remain isolated")
	fail := true
	oneCalls, twoCalls := 0, 0
	bindCompressionMock(t, parent, func(req *AIRequest) (string, error) {
		if strings.Contains(req.GetPrompt(), "PRIVATE_TWO") {
			twoCalls++
			if fail {
				return "", errors.New("temporary model failure")
			}
		} else {
			oneCalls++
		}
		return compressionMockSummary("private source processed"), nil
	})
	require.Error(t, parent.RecoverMemory(context.Background()))
	fail = false
	require.NoError(t, parent.RecoverMemory(context.Background()))
	require.NoError(t, parent.ArchiveMemoryBranch(first.Branch, first.BaseMaxID))
	require.NoError(t, parent.ArchiveMemoryBranch(second.Branch, second.BaseMaxID))
	require.NoError(t, parent.RecoverMemory(context.Background()))
	require.Equal(t, 1, oneCalls, "successful sources must not be repeated")
	require.Equal(t, 2, twoCalls, "only the failed source is retried")
	require.Empty(t, parent.sessionMemory.retrySnapshot())
}

func TestTaskCompletionWaitsFinalizationAndStopBypasses(t *testing.T) {
	for _, abort := range []bool{false, true} {
		task := NewStatefulTaskBase("full-task", "input", context.Background(), NewDummyEmitter(), true)
		task.SetStatus(AITaskState_Processing)
		first, last := task.DeferCompletion(), task.DeferCompletion()
		task.SetStatus(AITaskState_Completed)
		require.Equal(t, AITaskState_Processing, task.GetStatus())
		require.NoError(t, task.GetContext().Err())
		first()
		first()
		require.Equal(t, AITaskState_Processing, task.GetStatus())
		if abort {
			task.SetStatus(AITaskState_Aborted)
			require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
		}
		last()
		last()
		if abort {
			require.Equal(t, AITaskState_Aborted, task.GetStatus())
		} else {
			require.Equal(t, AITaskState_Completed, task.GetStatus())
		}
		require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
	}
}

func mustMarshalMemoryTimeline(t *testing.T, tl *Timeline) string {
	t.Helper()
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	return raw
}

func TestTimelineMemoryCheckpointFailurePreservesOriginal(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "memory-checkpoint", PersistentSession: "memory-checkpoint"}).Error)
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "KEEP_ORIGINAL_ON_STORAGE_ERROR")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("usable summary"), nil })
	cfg := tl.config.(*Config)
	cfg.DisableCreateDBRuntime = false
	cfg.PersistentSessionId = "memory-checkpoint"
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("memory-checkpoint", db)
	require.NoError(t, db.Close())
	_, err = tl.CompressOnce(compressionTestOptions())
	require.Error(t, err)
	require.Equal(t, []int64{1}, tl.GetTimelineItemIDs())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "KEEP_ORIGINAL_ON_STORAGE_ERROR")
	require.Nil(t, tl.compressedHead)
	require.Empty(t, tl.GetSessionMemoryCandidates())
	require.Len(t, tl.sessionMemory.retrySnapshot(), 1)
}

func TestTimelineMemoryCleanBranchAndRemapPreserveCoverage(t *testing.T) {
	parent := NewTimeline(nil, nil)
	child := NewPrivateTimeline(nil, nil)
	require.False(t, child.IsSessionTimeline())
	require.True(t, child.IsBranchTimeline())
	child.EnsureTaskUserInput("clean-agent", "PRIVATE_EXACT", func() int64 { return 9000 })
	require.NoError(t, parent.ArchiveMemoryBranch(child, 0))
	require.NoError(t, parent.ArchiveMemoryBranch(child, 0))
	require.Empty(t, RenderTimelineFrozenOpen(parent).Open)
	calls := 0
	bindCompressionMock(t, parent, func(req *AIRequest) (string, error) {
		calls++
		require.Contains(t, req.GetPrompt(), "PRIVATE_EXACT")
		return compressionMockSummary("private information considered"), nil
	})
	require.NoError(t, parent.RecoverMemory(context.Background()))
	require.NoError(t, parent.ArchiveMemoryBranch(child, 0))
	require.NoError(t, parent.RecoverMemory(context.Background()))
	require.Equal(t, 1, calls)
	parent.PushText(9001, "confirmed business fact")
	bindCompressionMock(t, parent, func(*AIRequest) (string, error) { calls++; return compressionMockSummary("completed fact"), nil })
	require.NoError(t, parent.FinalizeMemory(context.Background(), compressionTestOptions()))
	raw := mustMarshalMemoryTimeline(t, parent)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	next := int64(0)
	restored.ReassignIDs(func() int64 { next++; return next })
	bindCompressionMock(t, restored, func(*AIRequest) (string, error) {
		t.Fatal("unchanged restored source must not be summarized")
		return "", nil
	})
	require.NoError(t, restored.FinalizeMemory(context.Background()))
	require.Equal(t, 2, calls)
}

func TestTimelineMemoryDeliveryCheckpointFailureDoesNotAcknowledge(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: "memory-delivery", PersistentSession: "memory-delivery"}).Error)
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "ORIGINAL_BEFORE_DELIVERY_FAILURE")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
		return string(raw), err
	})
	cfg := tl.config.(*Config)
	cfg.DisableCreateDBRuntime = false
	cfg.PersistentSessionId = "memory-delivery"
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("memory-delivery", db)
	tl.RegisterSummaryCallback("lose-db-after-commit", func(TimelineSummaryEvent) { require.NoError(t, db.Close()) })
	var deliveredErr error
	tl.RegisterSessionMemoryCallback("delivery", func(event TimelineMemoryEvent) { deliveredErr = event.Err })
	require.NoError(t, tl.RequestMemoryFinalization())
	require.Error(t, tl.FinalizeMemory(context.Background(), compressionTestOptions()))
	require.Error(t, deliveredErr, "listeners must not treat an unjournaled source as ready for durable writes")
	require.True(t, tl.HasPendingMemoryFinalization())
	require.Len(t, tl.GetSessionMemoryCandidates(), 1)
	_, sources := tl.GetMemoryPersistenceBatch()
	require.Len(t, sources, 1)
	require.Error(t, tl.AcknowledgeMemoryPersistence(sources))
	require.Len(t, tl.sessionMemory.retrySnapshot(), 1, "failed receipt checkpoint must keep the recovery source")
	require.Contains(t, mustMarshalMemoryTimeline(t, tl), "ORIGINAL_BEFORE_DELIVERY_FAILURE")
}

func TestTimelineMemoryMissingRuntimeDoesNotRetireSource(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "KEEP_ORIGINAL_WITHOUT_RUNTIME")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("usable summary"), nil })
	cfg := tl.config.(*Config)
	cfg.DisableCreateDBRuntime = false
	cfg.PersistentSessionId = "missing-memory-runtime"
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("missing-memory-runtime", db)
	_, err = tl.CompressOnce(compressionTestOptions())
	require.ErrorContains(t, err, "has no runtime")
	require.Equal(t, []int64{1}, tl.GetTimelineItemIDs())
	require.Nil(t, tl.compressedHead)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "KEEP_ORIGINAL_WITHOUT_RUNTIME")
	require.Len(t, tl.sessionMemory.retrySnapshot(), 1)
}
