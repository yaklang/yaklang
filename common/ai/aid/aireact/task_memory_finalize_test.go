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

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type finalizationTestMemory struct {
	aicommon.MemoryTriage
	triage  atomic.Int32
	failed  atomic.Bool
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *finalizationTestMemory) HandleMemory(any) error { m.triage.Add(1); return nil }
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
	return nil
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
					if req.GetCallerLabel() == aicommon.CallerLabelTimelineCompress {
						compressionCalls.Add(1)
						require.Contains(t, req.GetPrompt(), "FULL_USER_TASK")
						name, params = "timeline-summary", map[string]any{"summary": "business task completed", "ratain_timeline_item_range": "", "memory_entities": []any{timelinePersistenceCandidate()}}
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
				task := aicommon.NewStatefulTaskBase("full-user", "FULL_USER_TASK: preserve explicit report requirements", ctx, r.Emitter, true)
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
				require.Equal(t, aicommon.AITaskState_Processing, task.GetStatus(), "completion must wait for the final save")
				require.NoError(t, task.GetContext().Err())
				close(memory.release)
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("full task did not finish")
				}
				require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
				require.EqualValues(t, 1, compressionCalls.Load())
				require.Zero(t, memory.triage.Load(), "automatic extraction must only use compression")
				require.Equal(t, failure, r.config.Timeline.HasPendingMemoryFinalization())
				if failure {
					archive, err := aicommon.MarshalTimeline(r.config.Timeline)
					require.NoError(t, err)
					require.Contains(t, archive, "FULL_USER_TASK", "failed save must retain its source")
					memory.failed.Store(false)
					require.NoError(t, aimem.CompleteTimelineMemory(r.config, ctx, task.GetOriginUserInput()))
					require.EqualValues(t, 1, compressionCalls.Load(), "save retry must not extract again")
					require.False(t, r.config.Timeline.HasPendingMemoryFinalization())
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
				if req.GetCallerLabel() != aicommon.CallerLabelTimelineCompress {
					return nil, fmt.Errorf("unexpected request: %s", req.GetCallerLabel())
				}
				calls.Add(1)
				require.Contains(t, req.GetPrompt(), "plan: ASYNC_MEMORY_TASK is finished")
				require.Contains(t, req.GetPrompt(), "FINAL_HOST_ARTIFACT")
				data, _ := json.Marshal(map[string]any{"@action": "timeline-summary", "summary": "final host record checked", "memory_entities": []any{timelinePersistenceCandidate()}})
				rsp := cfg.NewAIResponse()
				rsp.EmitOutputStream(strings.NewReader(string(data)))
				rsp.Close()
				return rsp, nil
			}
			r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithMemoryTriage(memory),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAICallback(model), aicommon.WithSpeedPriorityAICallback(model), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1))
			require.NoError(t, err)
			task := aicommon.NewStatefulTaskBase("async-memory", "ASYNC_MEMORY_TASK", ctx, r.Emitter, true)
			task.SetAsyncMode(true)
			task.SetStatus(aicommon.AITaskState_Processing)
			task.SetAsyncDeferCallback(func(err error) { require.NoError(t, err); task.SetStatus(aicommon.AITaskState_Completed) })
			r.setCurrentTask(task)
			r.persistTaskUserInput(task)
			r.config.HijackPERequest = func(context.Context, string) error { r.AddToTimeline("artifact", "FINAL_HOST_ARTIFACT"); return nil }
			done := make(chan struct{})
			r.AsyncPlanAndExecute(ctx, "ASYNC_MEMORY_TASK", func(err error) { require.NoError(t, err); close(done) })
			select {
			case <-memory.started:
			case <-ctx.Done():
				t.Fatal("async final save did not start")
			}
			require.Equal(t, aicommon.AITaskState_Processing, task.GetStatus())
			close(memory.release)
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("async task did not finish")
			}
			require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
			require.EqualValues(t, 1, calls.Load())
			require.Zero(t, memory.triage.Load())
			require.False(t, r.config.Timeline.HasPendingMemoryFinalization())
		})
	}
}
