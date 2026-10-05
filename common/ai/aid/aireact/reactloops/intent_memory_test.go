package reactloops

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type intentQueryMemory struct {
	aicommon.MemoryTriage
	calls atomic.Int32
	query func(string) (*aicommon.SearchMemoryResult, error)
}

func (m *intentQueryMemory) SearchMemoryWithoutAI(input any, _ int) (*aicommon.SearchMemoryResult, error) {
	query := m.query
	m.calls.Add(1)
	return query(input.(string))
}

func intentTestLoop(memory aicommon.MemoryTriage) *ReActLoop {
	return &ReActLoop{memoryTriage: memory, vars: omap.NewEmptyOrderedMap[string, any]()}
}

func intentResult(content string) *aicommon.SearchMemoryResult {
	return &aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{&aicommon.MemoryEntity{Id: content, Content: content, O_Score: .8, R_Score: .8, CreatedAt: time.Now().Add(-time.Hour)}}}
}

func TestIntentMemoryLateResultsStableSnapshotAndExplicitSearch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldStarted, oldRelease, oldReturned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(oldRelease)
	memory := &intentQueryMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage()}
	memory.query = func(query string) (*aicommon.SearchMemoryResult, error) {
		if query == "old goal" {
			close(oldStarted)
			<-oldRelease
			close(oldReturned)
			return intentResult("old memory"), nil
		}
		return intentResult("new memory"), nil
	}
	loop := intentTestLoop(memory)
	old := aicommon.NewStatefulTaskBase("task", "old goal", ctx, nil, true)
	loop.RecallMemoryForIntent(old, "v1")
	<-oldStarted
	newTask := aicommon.NewStatefulTaskBase("task", "new goal", ctx, nil, true)
	loop.RecallMemoryForIntent(newTask, "v2")
	require.Eventually(t, func() bool { return strings.Contains(loop.GetCurrentMemoriesContent(), "new memory") }, time.Second, time.Millisecond)
	snapshot := loop.GetCurrentMemoriesContent()
	// Explicit query returns data only and cannot replace/invalidate injection.
	_, err := memory.SearchMemoryWithoutAI("manual query", 1200)
	require.NoError(t, err)
	require.Equal(t, snapshot, loop.GetCurrentMemoriesContent())
	loop.RecallMemoryForIntent(newTask, "v2")
	require.EqualValues(t, 3, memory.calls.Load(), "same intent must not retrieve again")
	oldRelease <- struct{}{}
	<-oldReturned
	require.Never(t, func() bool { return loop.GetCurrentMemoriesContent() != snapshot }, 100*time.Millisecond, time.Millisecond, "late old recall must not replace the new snapshot")
	// A new empty/no-intent task invalidates even a result that has not published.
	loop.resetIntentMemory()
	require.Empty(t, loop.GetCurrentMemoriesContent())
	time.Sleep(20 * time.Millisecond)
	require.Empty(t, loop.GetCurrentMemoriesContent())
}

func TestIntentMemoryThirtyRoundLoop(t *testing.T) {
	for _, recognized := range []bool{false, true} {
		name := "without_intent"
		if recognized {
			name = "recognized_intent"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			memory := &intentQueryMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage(), query: func(string) (*aicommon.SearchMemoryResult, error) {
				return intentResult("stable recalled context"), nil
			}}
			var loop *ReActLoop
			var snapshot string
			requests := 0
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				requests++
				if recognized {
					require.Eventually(t, func() bool { return strings.Contains(loop.GetCurrentMemoriesContent(), "stable recalled context") }, time.Second, time.Millisecond)
					if requests == 1 {
						snapshot = loop.GetCurrentMemoriesContent()
					}
					require.Equal(t, snapshot, loop.GetCurrentMemoriesContent())
					if requests > 1 {
						require.Contains(t, req.GetPrompt(), "stable recalled context")
					}
				} else {
					require.Empty(t, loop.GetCurrentMemoriesContent())
				}
				response := c.NewAIResponse()
				response.EmitOutputStream(bytes.NewBufferString(`{"@action":"advance"}`))
				response.Close()
				return response, nil
			}))
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			steps := 0
			var err error
			loop, err = NewReActLoop("intent-memory-30", invoker, WithFunctionCallMode(false), WithAllowToolCall(false), WithAllowRAG(false), WithAllowAIForge(false), WithAllowPlanAndExec(false), WithAllowUserInteract(false), WithDisableLoopPerception(true), WithDisablePeriodicVerification(true), WithDisableIncreaseIteration(true), WithMemoryTriage(memory),
				WithInitTask(func(l *ReActLoop, task aicommon.AIStatefulTask, _ *InitTaskOperator) {
					if recognized {
						l.RecallMemoryForIntent(task, "one recognized intent")
					}
				}),
				WithRegisterLoopAction("require_tool", "unused tool routing", nil, nil, nil),
				WithRegisterLoopAction("advance", "advance test", nil, nil, func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
					steps++
					if steps == 30 {
						op.Exit()
					} else {
						op.Continue()
					}
				}))
			require.NoError(t, err)
			require.NoError(t, loop.Execute("task", ctx, "execute the task"))
			require.Equal(t, 30, requests)
			expected := int32(0)
			if recognized {
				expected = 1
			}
			require.Equal(t, expected, memory.calls.Load())
			t.Logf("model_rounds=%d memory_queries=%d snapshot_stable=true", requests, memory.calls.Load())
		})
	}
}

func TestIntentMemoryEmptyAndConservativeSelection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	memory := &intentQueryMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage()}
	memory.query = func(string) (*aicommon.SearchMemoryResult, error) {
		return &aicommon.SearchMemoryResult{}, nil
	}
	loop := intentTestLoop(memory)
	task := aicommon.NewStatefulTaskBase("task", "goal", ctx, nil, true)
	loop.RecallMemoryForIntent(task, "intent")
	require.Eventually(t, func() bool { return memory.calls.Load() == 1 }, time.Second, time.Millisecond)
	for i := 0; i < 30; i++ {
		loop.RecallMemoryForIntent(task, "intent")
		require.Empty(t, loop.GetCurrentMemoriesContent())
	}
	require.EqualValues(t, 1, memory.calls.Load())
	past := time.Now().Add(-time.Hour)
	memory.query = func(string) (*aicommon.SearchMemoryResult, error) {
		return &aicommon.SearchMemoryResult{Memories: []*aicommon.MemoryEntity{
			{Content: "untrusted", O_Score: .1, R_Score: 1, P_Score: 1},
			{Content: "unrelated", O_Score: 1, R_Score: .1},
			{Content: "expired", O_Score: 1, R_Score: 1, ExpiresAt: &past},
			{Id: "good", Content: "useful memory", O_Score: .8, R_Score: .8},
		}}, nil
	}
	loop.RecallMemoryForIntent(task, "changed intent")
	require.Eventually(t, func() bool { return strings.Contains(loop.GetCurrentMemoriesContent(), "useful memory") }, time.Second, time.Millisecond)
	snapshot := loop.GetCurrentMemoriesContent()
	require.NotContains(t, snapshot, "untrusted")
	require.NotContains(t, snapshot, "unrelated")
	require.NotContains(t, snapshot, "expired")
	time.Sleep(1100 * time.Millisecond)
	require.Equal(t, snapshot, loop.GetCurrentMemoriesContent(), "relative age must not churn prompts")
}
