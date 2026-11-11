package loopinfra

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

type toolBatchExitChildBuilder func(*reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error)

func (f toolBatchExitChildBuilder) Build(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
	return f(p)
}

// Exercise the real parent loop: direct answer is requested by approval, then
// delivered by the normal primary decision prompt before a final finish.
func TestToolBatchUserAnswerCancelsChildrenAndExitsParent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls atomic.Int32
	var loop *reactloops.ReActLoop
	var cancelledBeforeAnswer bool
	var err error
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			var body string
			switch calls.Add(1) {
			case 1:
				body = `{"@action":"start_child"}`
			case 2:
				body = `{"@action":"batch_review"}`
			case 3:
				jobs, inspectErr := loop.GetSubAgentManager().Inspect(nil)
				cancelledBeforeAnswer = inspectErr == nil && len(jobs) == 1 && jobs[0].State == "cancelled"
				body = `{"@action":"directly_answer","answer_payload":"已停止工具调用，并根据已有信息回答。"}`
			case 4:
				body = `{"@action":"finish"}`
			default:
				return nil, fmt.Errorf("parent continued after explicit user exit")
			}
			response := c.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(body))
			response.Close()
			return response, nil
		}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	original := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = original }()
	aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		child := mock.NewMockInvoker(ctx)
		child.SetConfig(aicommon.NewConfig(ctx, options...))
		return child, nil
	}
	started := make(chan struct{}, 1)
	builder := toolBatchExitChildBuilder(func(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
		child := reactloops.NewMinimalReActLoop(p.Invoker.GetConfig(), p.Invoker)
		reactloops.WithInitTask(func(_ *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			started <- struct{}{}
			<-task.GetContext().Done()
			op.Done()
		})(child)
		return child, nil
	})
	loop, err = reactloops.NewReActLoop("tool-batch-user-exit", inv,
		reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
		reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true),
		reactloops.WithRegisterLoopAction("start_child", "start background work", nil, nil,
			func(loop *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				_, err := loop.SubmitSubAgents(op.GetTask(), []reactloops.SubAgentJob{{Identifier: "active-child"}},
					reactloops.SubAgentOptions{TimelineMode: reactloops.SubAgentTimelineClean, LoopBuilder: builder}, "child")
				if err != nil {
					op.Fail(err)
					return
				}
				select {
				case <-started:
					op.Continue()
				case <-ctx.Done():
					op.Fail(ctx.Err())
				}
			}),
		reactloops.WithRegisterLoopAction("batch_review", "user selects direct answer during batch review", nil, nil,
			func(loop *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				handleToolBatchActionResult(loop, op.GetContext(), inv, &aicommon.ToolCallGroupRequest{},
					&aicommon.ToolCallGroupResult{DirectlyAnswer: true}, nil, op)
			}))
	require.NoError(t, err)
	require.NoError(t, loop.Execute("tool-batch-user-exit", ctx, "Investigate using parallel workers and tools"))
	require.True(t, cancelledBeforeAnswer, "owned work must stop before generating the user-requested answer")
	require.EqualValues(t, 4, calls.Load(), "the normal primary loop must answer before final finish")
	jobs, err := loop.GetSubAgentManager().Inspect(nil)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, "cancelled", jobs[0].State)
}
