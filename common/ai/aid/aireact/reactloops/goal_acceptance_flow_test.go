package reactloops

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type goalAcceptanceTimelineInvoker struct {
	*mock.MockInvoker
	cfg *aicommon.Config
}

func TestGoalDeadlineStartsAtConfigurationAndFinishOnlyReadsIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEnableGoalMode(true), aicommon.WithGoalMinIterations(0), aicommon.WithGoalDurationSeconds(10800))
	loop := NewMinimalReActLoop(cfg, mock.NewMockInvoker(ctx))
	deadline := cfg.GetGoalDeadline()
	require.WithinDuration(t, time.Now().Add(3*time.Hour), deadline, time.Second)
	require.False(t, loop.ShouldBlockFinishAtIteration(1))
	require.True(t, loop.ShouldBlockFinishByDeadline())
	require.Equal(t, deadline, cfg.GetGoalDeadline())
	// A delayed first finish must observe the expired deadline, not start a new one.
	cfg.GoalDeadline = time.Now().Add(-time.Minute)
	expired := cfg.GetGoalDeadline()
	require.False(t, loop.ShouldBlockFinishByDeadline())
	require.Equal(t, expired, cfg.GetGoalDeadline())
}

func (i *goalAcceptanceTimelineInvoker) AddToTimeline(entry, content string) {
	i.cfg.Timeline.PushText(i.cfg.AcquireId(), "[%s]:\n%s", entry, content)
}

func TestGoalAcceptanceRunsOneMainModelReviewTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const criteria = "deliver a verified report with concrete evidence"
	var calls atomic.Int32
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(0), // criteria-only mode must reach review on its first finish
		aicommon.WithGoalAcceptanceCriteria(criteria),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			call := calls.Add(1)
			prompt := req.GetPrompt()
			switch call {
			case 1:
				require.NotContains(t, prompt, "【必须执行 Goal 验收】")
			case 2:
				require.Contains(t, prompt, "【必须执行 Goal 验收】")
				require.Contains(t, prompt, criteria)
			default:
				t.Fatalf("unexpected extra model call %d", call)
			}
			response := c.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish"}`))
			response.Close()
			return response, nil
		}),
	)
	baseInvoker := mock.NewMockInvoker(ctx)
	baseInvoker.SetConfig(cfg)
	invoker := &goalAcceptanceTimelineInvoker{MockInvoker: baseInvoker, cfg: cfg}
	loop, err := NewReActLoop("goal-acceptance-flow", invoker,
		WithFunctionCallMode(false),
		WithAllowToolCall(false),
		WithAllowRAG(false),
		WithAllowAIForge(false),
		WithAllowPlanAndExec(false),
		WithAllowUserInteract(false),
		WithRegisterLoopAction("require_tool", "unused tool routing", nil, nil, nil),
		WithDisableLoopPerception(true),
		WithDisablePeriodicVerification(true),
		WithDisableIncreaseIteration(true),
	)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("goal-acceptance-task", "finish after review", ctx, cfg.GetEmitter(), true)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.EqualValues(t, 2, calls.Load(), "the first finish must enter exactly one acceptance review turn")
	require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
}

func TestGoalAcceptanceWorkActionExitRestoresNormalMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const criteria = "deliver a verified report with concrete evidence"
	var calls atomic.Int32
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(1),
		aicommon.WithGoalAcceptanceCriteria(criteria),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			call := calls.Add(1)
			prompt := req.GetPrompt()
			var output string
			switch call {
			case 1:
				require.NotContains(t, prompt, "【必须执行 Goal 验收】")
				output = `{"@action":"finish"}`
			case 2:
				require.Contains(t, prompt, "【必须执行 Goal 验收】")
				output = `{"@action":"review_exit_action","identifier":"review_exit_action"}`
			case 3:
				// The review hint remains in Timeline, but promptMode was reset.
				// This finish must therefore arm a fresh review instead of exiting.
				output = `{"@action":"finish"}`
			case 4:
				require.Contains(t, prompt, "【必须执行 Goal 验收】",
					"a later finish must receive a fresh acceptance review")
				output = `{"@action":"finish"}`
			default:
				t.Fatalf("unexpected extra model call %d", call)
			}
			response := c.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(output))
			response.Close()
			return response, nil
		}),
	)
	baseInvoker := mock.NewMockInvoker(ctx)
	baseInvoker.SetConfig(cfg)
	invoker := &goalAcceptanceTimelineInvoker{MockInvoker: baseInvoker, cfg: cfg}
	loop, err := NewReActLoop("goal-acceptance-work", invoker,
		WithFunctionCallMode(false),
		WithAllowToolCall(false),
		WithAllowRAG(false),
		WithAllowAIForge(false),
		WithAllowPlanAndExec(false),
		WithAllowUserInteract(false),
		WithRegisterLoopAction("require_tool", "unused tool routing", nil, nil, nil),
		WithRegisterLoopAction("review_exit_action", "exercise non-finish termination during review", nil, nil,
			func(_ *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
				op.Exit()
			}),
		WithDisableLoopPerception(true),
		WithDisablePeriodicVerification(true),
		WithDisableIncreaseIteration(true),
	)
	require.NoError(t, err)

	task := aicommon.NewStatefulTaskBase("goal-acceptance-work-task", "review then continue", ctx, cfg.GetEmitter(), true)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.EqualValues(t, 4, calls.Load())
	require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
}
