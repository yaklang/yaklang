package aireact

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type queueBlockingMemory struct {
	aicommon.MemoryTriage
	started          chan int
	release          chan struct{}
	once             sync.Once
	extractionCalls  atomic.Int32
	persistenceCalls atomic.Int32
}

// The durable handoff must precede task completion, even if the backend never
// returns. Bursts share one pending pass; disconnect leaves that work on disk.
func TestUserTaskMemoryBackgroundCheckpointAndRecovery(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprintf("disconnect=%v", disconnect), func(t *testing.T) {
			db, err := utils.CreateTempTestDatabaseInMemory()
			require.NoError(t, err)
			defer db.Close()
			require.NoError(t, db.AutoMigrate(&schema.AIAgentRuntime{}).Error)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			memory := &queueBlockingMemory{MemoryTriage: aimem.NewMockMemoryTriage(), started: make(chan int, 1), release: make(chan struct{})}
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true),
				aicommon.WithMemoryTriage(memory), aicommon.WithWorkdir(t.TempDir()))
			// Bind the isolated DB after configuration so no global runtime is used.
			cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(cfg.Id, db)
			cfg.PersistentSessionId = "background-memory-checkpoint"
			cfg.DisableCreateDBRuntime = false
			require.NoError(t, db.Create(&schema.AIAgentRuntime{Uuid: cfg.Id, PersistentSession: cfg.PersistentSessionId}).Error)
			r := &ReAct{config: cfg, Emitter: cfg.Emitter, memoryTriage: memory, lifecycleWG: new(sync.WaitGroup)}
			defer func() { cancel(); r.WaitLifecycleStopped() }()
			restoreFinalizationCandidates(t, r)
			finishTask := func(input string) {
				task := aicommon.NewStatefulTaskBase(input, input, ctx, r.Emitter, true)
				task.SetStatus(aicommon.AITaskState_Processing)
				r.persistTaskUserInput(task)
				release := r.beginUserTaskMemory(task)
				task.SetStatus(aicommon.AITaskState_Completed)
				r.completeUserTaskMemory(task)
				release()
				require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
			}
			finishTask("first durable question")
			select {
			case <-memory.started:
			case <-ctx.Done():
				t.Fatal("background persistence did not start")
			}
			for i := 0; i < 10; i++ {
				finishTask(fmt.Sprintf("queued durable question %d", i))
			}
			require.EqualValues(t, 1, memory.persistenceCalls.Load())
			var record schema.AIAgentRuntime
			require.NoError(t, db.Where("uuid = ?", cfg.Id).First(&record).Error)
			raw, err := strconv.Unquote(record.QuotedTimeline)
			require.NoError(t, err)
			require.Contains(t, raw, "first durable question")
			require.Contains(t, raw, "queued durable question 9")
			restored, err := aicommon.UnmarshalTimeline(raw)
			require.NoError(t, err)
			require.True(t, restored.HasPendingMemoryFinalization())
			require.Len(t, restored.GetSessionMemoryCandidates(), 1)
			if disconnect {
				cancel()
				r.WaitLifecycleStopped()
				require.EqualValues(t, 1, memory.persistenceCalls.Load(), "cancelled queued work must be left for recovery")
				// The same durable recovery path can drain a restored checkpoint
				// using a fresh session context, with no new extraction request.
				recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer recoveryCancel()
				recoveredMemory := &finalizationTestMemory{MemoryTriage: aimem.NewMockMemoryTriage()}
				require.NoError(t, restored.PrepareMemoryRecovery())
				require.NoError(t, aimem.PersistPendingTimelineMemory(restored, recoveredMemory, recoveryCtx))
				require.EqualValues(t, 1, recoveredMemory.saved.Load())
				require.Zero(t, recoveredMemory.triage.Load())
				require.False(t, restored.HasPendingMemoryFinalization())
			} else {
				close(memory.release)
				r.waitMemoryFinalization()
				require.EqualValues(t, 2, memory.persistenceCalls.Load(), "ten completions during a save should coalesce into one further pass")
			}
		})
	}
}

func (m *queueBlockingMemory) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	m.persistenceCalls.Add(1)
	m.once.Do(func() { m.started <- len(candidates) })
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *queueBlockingMemory) HandleMemory(any) error {
	m.extractionCalls.Add(1)
	return fmt.Errorf("unexpected extraction during completion")
}

func (m *queueBlockingMemory) AddRawText(string) ([]*aicommon.MemoryEntity, error) {
	m.extractionCalls.Add(1)
	return nil, fmt.Errorf("unexpected extraction during completion")
}

// Exercise the real input/task queue with a blocked persistence backend. Both
// tasks must finish before the backend is released, without extra model calls.
func TestUserTaskMemoryFinalizationDoesNotBlockNextQueuedTask(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, candidates := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/candidates=%v", native, candidates), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				memory := &queueBlockingMemory{
					MemoryTriage: aimem.NewMockMemoryTriage(),
					started:      make(chan int, 1), release: make(chan struct{}),
				}
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(memory.release) }) }
				defer release()
				var decisions, compressionCalls, auxiliaryCalls atomic.Int32
				var r *ReAct
				var err error
				startedTasks := make(chan aicommon.AIStatefulTask, 2)
				secondStarted := make(chan struct{}, 1)
				model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
						compressionCalls.Add(1)
						return nil, fmt.Errorf("finish must not start context compression")
					}
					if !strings.HasPrefix(req.GetCallerLabel(), "react-loop:") {
						auxiliaryCalls.Add(1)
						return nil, fmt.Errorf("unexpected auxiliary request: %s", req.GetCallerLabel())
					}
					call := decisions.Add(1)
					if call == 1 || call == 3 {
						startedTasks <- r.GetCurrentTask()
					}
					if call == 3 {
						require.Contains(t, req.GetPrompt(), "Complete question one.", "the next task must still inherit the session timeline")
						secondStarted <- struct{}{}
					}
					if call%2 == 1 {
						return compressionActionResponse(t, cfg, req, "directly_answer", map[string]any{"answer_payload": "Task work is complete."}), nil
					}
					return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
				}
				r, err = NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithAICallback(model), aicommon.WithSpeedPriorityAICallback(model))
				require.NoError(t, err)
				defer func() { release(); cancel(); r.WaitLifecycleStopped() }()
				// Attach a controlled storage boundary after startup, excluding backend
				// initialization/recovery from the measurement of task finalization.
				r.memoryTriage, r.config.MemoryTriage = memory, memory
				if candidates {
					restoreFinalizationCandidates(t, r)
				}
				require.NoError(t, r.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Complete question one."}))
				select {
				case count := <-memory.started:
					if candidates {
						require.Equal(t, 1, count)
					} else {
						require.Zero(t, count, "even an empty batch reaches persistence to recover pending work")
					}
				case <-ctx.Done():
					t.Fatal("first task did not reach final persistence")
				}
				first := <-startedTasks
				require.EqualValues(t, 2, decisions.Load(), "the first task has already answered and called finish")
				waitStart := time.Now()
				require.NoError(t, r.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Now answer question two."}))
				select {
				case <-secondStarted:
				case <-ctx.Done():
					t.Fatal("second task is blocked by the first task's memory persistence")
				}
				second := <-startedTasks
				require.Eventually(t, second.IsFinished, 3*time.Second, 10*time.Millisecond)
				require.Equal(t, aicommon.AITaskState_Completed, first.GetStatus())
				require.Equal(t, aicommon.AITaskState_Completed, second.GetStatus())
				require.Zero(t, compressionCalls.Load())
				require.Zero(t, auxiliaryCalls.Load())
				require.Zero(t, memory.extractionCalls.Load())
				require.EqualValues(t, 1, memory.persistenceCalls.Load(), "there must be only one active background drain")
				require.True(t, r.config.Timeline.HasPendingMemoryFinalization())
				t.Logf("second task finished in %s while memory persistence was still blocked", time.Since(waitStart).Round(time.Millisecond))
				release()
				r.waitMemoryFinalization()
				require.EqualValues(t, 2, memory.persistenceCalls.Load(), "completion during a save must schedule a fresh pass")
			})
		}
	}
}
