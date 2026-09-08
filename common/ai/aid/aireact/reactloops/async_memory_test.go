package reactloops

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type blockedRecallMemory struct {
	aicommon.MemoryTriage
	release chan struct{}
	started chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (m *blockedRecallMemory) SearchMemoryWithoutAI(any, int) (*aicommon.SearchMemoryResult, error) {
	m.calls.Add(1)
	m.once.Do(func() { close(m.started) })
	<-m.release
	return &aicommon.SearchMemoryResult{
		Memories: []*aicommon.MemoryEntity{{Id: "async-memory", Content: "remembered asynchronous context"}},
	}, nil
}

func TestAsyncMemoryDoesNotBlockIterationsAndReachesLaterPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	memory := &blockedRecallMemory{MemoryTriage: aicommon.NewNoOpMemoryTriage(), release: make(chan struct{}), started: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(memory.release) }) }
	defer release()
	prompts := make(chan string, 1)
	responses := make(chan struct{}, 1)
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			select {
			case prompts <- req.GetPrompt():
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			select {
			case <-responses:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			response := c.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"advance"}`))
			response.Close()
			return response, nil
		}),
	)
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(cfg)
	steps := 0
	loop, err := NewReActLoop("async-memory-test", invoker,
		WithAllowToolCall(false),
		WithAllowRAG(false),
		WithAllowAIForge(false),
		WithAllowPlanAndExec(false),
		WithAllowUserInteract(false),
		WithRegisterLoopAction("require_tool", "unused tool routing", nil, nil, nil),
		WithMemoryTriage(memory),
		WithDisableLoopPerception(true),
		WithDisablePeriodicVerification(true),
		WithDisableIncreaseIteration(true),
		WithRegisterLoopAction("advance", "advance the test", nil, nil,
			func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
				steps++
				if steps == 3 {
					op.Exit()
				} else {
					op.Continue()
				}
			}),
	)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- loop.Execute("test-task", ctx, "perform the task") }()
	nextPrompt := func() string {
		t.Helper()
		select {
		case prompt := <-prompts:
			return prompt
		case err := <-done:
			t.Fatalf("loop stopped before producing the next prompt: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatal("memory search blocked the main loop")
		}
		return ""
	}
	require.NotContains(t, nextPrompt(), "remembered asynchronous context")
	select {
	case <-memory.started:
	case <-time.After(time.Second):
		t.Fatal("recall did not start")
	}
	responses <- struct{}{}
	require.NotContains(t, nextPrompt(), "remembered asynchronous context")
	require.EqualValues(t, 1, memory.calls.Load(), "iterations must not stack up concurrent quick searches")

	release()
	require.Eventually(t, func() bool {
		return strings.Contains(loop.GetCurrentMemoriesContent(), "remembered asynchronous context")
	}, 2*time.Second, 10*time.Millisecond)
	responses <- struct{}{}
	require.Contains(t, nextPrompt(), "remembered asynchronous context")
	responses <- struct{}{}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not finish")
	}
}
