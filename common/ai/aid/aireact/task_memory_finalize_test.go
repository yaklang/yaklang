package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type finalizationTestMemory struct {
	aicommon.MemoryTriage
	triage  atomic.Int32
	saved   atomic.Int32
	failed  atomic.Bool
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *finalizationTestMemory) HandleMemory(any) error { m.triage.Add(1); return nil }
func (m *finalizationTestMemory) AddRawText(string) ([]*aicommon.MemoryEntity, error) {
	m.triage.Add(1)
	return nil, fmt.Errorf("unexpected memory extraction at task completion")
}
func (m *finalizationTestMemory) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	if len(candidates) == 0 {
		return nil
	}
	if m.started != nil {
		m.once.Do(func() { close(m.started) })
		select {
		case <-m.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.failed.Load() {
		return fmt.Errorf("injected persistence failure")
	}
	m.saved.Add(1)
	return nil
}

// Completion drains previously extracted candidates; it no longer runs the
// removed auxiliary compressor to manufacture candidates from the live tail.
func restoreFinalizationCandidates(t *testing.T, r *ReAct) {
	t.Helper()
	raw, err := aicommon.MarshalTimeline(r.config.Timeline)
	require.NoError(t, err)
	var snapshot map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &snapshot))
	snapshot["session_memory_candidates"], err = json.Marshal([]any{timelinePersistenceCandidate()})
	require.NoError(t, err)
	rawSnapshot, err := json.Marshal(snapshot)
	require.NoError(t, err)
	timeline, err := aicommon.UnmarshalTimeline(string(rawSnapshot))
	require.NoError(t, err)
	timeline.SoftBindConfig(r.config, nil)
	r.config.Timeline = timeline
	require.Len(t, timeline.GetSessionMemoryCandidates(), 1)
}

func TestUserTaskMemoryCompletionWithoutCandidatesDoesNotExtract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	memory := &finalizationTestMemory{MemoryTriage: aimem.NewMockMemoryTriage()}
	var calls atomic.Int32
	r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithMemoryTriage(memory),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls.Add(1)
			return nil, fmt.Errorf("completion must not create a memory extraction request")
		}))
	require.NoError(t, err)
	source := uuid.NewString()
	task := aicommon.NewStatefulTaskBase("no-candidates", source, ctx, r.Emitter, true)
	r.persistTaskUserInput(task)
	r.completeUserTaskMemory(task)
	r.waitMemoryFinalization()
	require.Zero(t, calls.Load())
	require.Zero(t, memory.triage.Load())
	require.Zero(t, memory.saved.Load())
	require.Empty(t, r.config.Timeline.GetSessionMemoryCandidates())
	require.True(t, r.config.Timeline.HasPendingMemoryFinalization())
	archive, err := aicommon.MarshalTimeline(r.config.Timeline)
	require.NoError(t, err)
	require.Contains(t, archive, source)
}

func TestUserTaskMemoryNormalCompletionAndFailedSave(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/failure=%v", native, failure), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
				defer cancel()
				memory := &finalizationTestMemory{MemoryTriage: aimem.NewMockMemoryTriage(), started: make(chan struct{}), release: make(chan struct{})}
				memory.failed.Store(failure)
				var loopCalls, compressionCalls atomic.Int32
				model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					name, params := "finish", map[string]any{}
					if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
						compressionCalls.Add(1)
						return nil, fmt.Errorf("task finalization must not request timeline compression")
					} else if strings.HasPrefix(req.GetCallerLabel(), "react-loop:") {
						if loopCalls.Add(1) == 1 {
							name, params = "directly_answer", map[string]any{"answer_payload": "checked the supplied material"}
						}
					} else {
						return nil, fmt.Errorf("unexpected auxiliary request: %s", req.GetCallerLabel())
					}
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					rsp := cfg.NewAIResponse()
					if wire.ToolCallCallback != nil {
						raw, _ := json.Marshal(params)
						wire.ToolCallCallback([]*aispec.ToolCall{{ID: "memory-finalization-call", Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(raw)}}})
						wire.FinishReasonCallback("tool_calls", nil)
					} else {
						params["@action"] = name
						raw, _ := json.Marshal(params)
						rsp.EmitOutputStream(strings.NewReader(string(raw)))
					}
					rsp.Close()
					return rsp, nil
				}
				r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithMemoryTriage(memory),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithAICallback(model), aicommon.WithSpeedPriorityAICallback(model))
				require.NoError(t, err)
				restoreFinalizationCandidates(t, r)
				taskSource := uuid.NewString()
				task := aicommon.NewStatefulTaskBase("full-user", taskSource+": preserve explicit report requirements", ctx, r.Emitter, true)
				r.setCurrentTask(task)
				r.persistTaskUserInput(task)
				task.SetStatus(aicommon.AITaskState_Processing)
				done := make(chan struct{})
				go func() { r.processReActTask(task); close(done) }()
				select {
				case <-memory.started:
				case <-ctx.Done():
					t.Fatal("final memory save did not start")
				}
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("task completion must not wait for memory indexing")
				}
				require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
				require.ErrorIs(t, task.GetContext().Err(), context.Canceled)
				require.Zero(t, memory.saved.Load(), "task finishes while the memory backend is still blocked")
				close(memory.release)
				r.waitMemoryFinalization()
				require.Zero(t, compressionCalls.Load(), "completion only persists existing candidates")
				require.Zero(t, memory.triage.Load(), "completion must not start another memory extraction")
				require.True(t, r.config.Timeline.HasPendingMemoryFinalization(), "the unextracted live tail must remain pending")
				if !failure {
					require.EqualValues(t, 1, memory.saved.Load())
				}
				if failure {
					archive, err := aicommon.MarshalTimeline(r.config.Timeline)
					require.NoError(t, err)
					require.Contains(t, archive, taskSource, "failed save must retain its source")
					require.Zero(t, memory.saved.Load())
					memory.failed.Store(false)
					require.NoError(t, aimem.CompleteTimelineMemory(r.config, ctx, task.GetOriginUserInput()))
					require.Zero(t, compressionCalls.Load(), "save retry must not compress or extract")
					require.EqualValues(t, 1, memory.saved.Load())
					require.True(t, r.config.Timeline.HasPendingMemoryFinalization(), "saving old candidates does not cover new task records")
				}
			})
		}
	}
}

func TestUserTaskMemoryReviewWaitAndDisconnect(t *testing.T) {
	for _, reviewWait := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		memory := &finalizationTestMemory{MemoryTriage: aimem.NewMockMemoryTriage()}
		r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithMemoryTriage(memory), aicommon.WithDisableCreateDBRuntime(true))
		require.NoError(t, err)
		task := aicommon.NewStatefulTaskBase("interrupted", "source before disconnect", ctx, r.Emitter, true)
		r.persistTaskUserInput(task)
		release := r.beginUserTaskMemory(task)
		if reviewWait {
			r.memoryContinuations.Store(task.GetId(), true)
		}
		cancel()
		release()
		require.Equal(t, !reviewWait, r.config.Timeline.HasPendingMemoryFinalization())
		require.Zero(t, memory.triage.Load())
		archive, err := aicommon.MarshalTimeline(r.config.Timeline)
		require.NoError(t, err)
		require.Contains(t, archive, "source before disconnect")
	}
}

func TestUserTaskMemoryAsyncIncludesFinalHostRecords(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			memory := &finalizationTestMemory{MemoryTriage: aimem.NewMockMemoryTriage(), started: make(chan struct{}), release: make(chan struct{})}
			var calls atomic.Int32
			model := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				calls.Add(1)
				return nil, fmt.Errorf("async completion must not request AI: %s", req.GetCallerLabel())
			}
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithMemoryTriage(memory),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAICallback(model), aicommon.WithSpeedPriorityAICallback(model), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1))
			require.NoError(t, err)
			restoreFinalizationCandidates(t, r)
			taskName, taskSource, artifact := uuid.NewString(), uuid.NewString(), uuid.NewString()
			task := aicommon.NewStatefulTaskBase(taskName, taskSource, ctx, r.Emitter, true)
			task.SetAsyncMode(true)
			task.SetStatus(aicommon.AITaskState_Processing)
			task.SetAsyncDeferCallback(func(err error) { require.NoError(t, err); task.SetStatus(aicommon.AITaskState_Completed) })
			r.setCurrentTask(task)
			r.persistTaskUserInput(task)
			r.config.HijackPERequest = func(context.Context, string) error { r.AddToTimeline("artifact", artifact); return nil }
			done := make(chan struct{})
			r.AsyncPlanAndExecute(ctx, taskName, func(err error) { require.NoError(t, err); close(done) })
			select {
			case <-memory.started:
			case <-ctx.Done():
				t.Fatal("async final save did not start")
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("async task completion must not wait for memory indexing")
			}
			require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
			require.Zero(t, memory.saved.Load())
			archive, err := aicommon.MarshalTimeline(r.config.Timeline)
			require.NoError(t, err)
			require.Contains(t, archive, "plan: "+taskName+" is finished")
			require.Contains(t, archive, artifact, "host records must be present before final persistence")
			close(memory.release)
			r.waitMemoryFinalization()
			require.Zero(t, calls.Load())
			require.EqualValues(t, 1, memory.saved.Load())
			require.Zero(t, memory.triage.Load())
			require.True(t, r.config.Timeline.HasPendingMemoryFinalization(), "final host records still need extraction")
		})
	}
}
