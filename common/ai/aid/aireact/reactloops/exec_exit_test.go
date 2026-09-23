package reactloops

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestExecuteWithExistedTask_PanicReturnsErrorAndPreservesTerminalStatus(t *testing.T) {
	for _, scenario := range []string{"ordinary", "active_child", "user_cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					response := c.NewAIResponse()
					response.EmitOutputStream(bytes.NewBufferString(`{"@action":"panic_action"}`))
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
			task := aicommon.NewStatefulTaskBase("panic-parent", "investigate incident", ctx, cfg.GetEmitter(), true)
			loop, err := NewReActLoop("panic-exit", inv,
				WithAllowToolCall(false), WithAllowRAG(false), WithAllowAIForge(false), WithAllowPlanAndExec(false), WithAllowUserInteract(false),
				WithRegisterLoopAction("require_tool", "unused tool routing", nil, nil, nil),
				WithDisableLoopPerception(true), WithDisablePeriodicVerification(true), WithDisableIncreaseIteration(true),
				WithRegisterLoopAction("panic_action", "exercise failure cleanup", nil, nil,
					func(loop *ReActLoop, _ *aicommon.Action, op *LoopActionHandlerOperator) {
						if scenario == "active_child" {
							started := make(chan string, 1)
							_, err := loop.SubmitSubAgents(task, []SubAgentJob{{Identifier: "child"}},
								SubAgentOptions{TimelineMode: SubAgentTimelineClean, LoopBuilder: blockingSubAgentBuilder(started, make(chan struct{}), "unused")}, "child")
							if err != nil {
								op.Fail(err)
								return
							}
							select {
							case <-started:
							case <-ctx.Done():
								op.Fail(ctx.Err())
								return
							}
						}
						if scenario == "user_cancelled" {
							task.SetUserCancelled()
							task.SetStatus(aicommon.AITaskState_Skipped)
						}
						panic("action failed unexpectedly")
					}))
			require.NoError(t, err)
			err = loop.ExecuteWithExistedTask(task)
			require.ErrorContains(t, err, "action failed unexpectedly")
			if scenario == "user_cancelled" {
				require.Equal(t, aicommon.AITaskState_Skipped, task.GetStatus())
				require.NotContains(t, task.GetResult(), "[Error]")
			} else {
				require.Equal(t, aicommon.AITaskState_Aborted, task.GetStatus())
				require.Contains(t, task.GetResult(), "action failed unexpectedly")
			}
			if scenario == "active_child" {
				jobs, err := loop.GetSubAgentManager().Inspect(nil)
				require.NoError(t, err)
				require.Len(t, jobs, 1)
				require.Equal(t, "cancelled", jobs[0].State)
			}
		})
	}
}

func TestExecuteWithExistedTask_InitDoneStillRecordsRootUserInput(t *testing.T) {
	ctx := context.Background()
	invoker := newTimelineCapturingInvoker(ctx)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true))
	invoker.SetConfig(cfg)

	loop := NewMinimalReActLoop(cfg, invoker)
	WithInitTask(func(_ *ReActLoop, _ aicommon.AIStatefulTask, op *InitTaskOperator) {
		op.Done()
	})(loop)

	task := aicommon.NewStatefulTaskBase("root-audit", "开始审计", ctx, nil, true)
	require.NoError(t, loop.ExecuteWithExistedTask(task))

	var hits []timelineEntry
	for _, entry := range invoker.Entries() {
		if entry.Tag == aicommon.TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT {
			hits = append(hits, entry)
		}
	}
	require.Len(t, hits, 1)
	require.Equal(t, "开始审计", hits[0].Content)
}

func TestExecuteWithExistedTask_SubtaskDoesNotRepeatParentUserInput(t *testing.T) {
	ctx := context.Background()
	invoker := newTimelineCapturingInvoker(ctx)
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true))
	invoker.SetConfig(cfg)

	loop := NewMinimalReActLoop(cfg, invoker)
	WithInitTask(func(_ *ReActLoop, _ aicommon.AIStatefulTask, op *InitTaskOperator) {
		op.Done()
	})(loop)

	parent := aicommon.NewStatefulTaskBase("root-audit", "开始审计", ctx, nil, true)
	child := aicommon.NewSubTaskBase(parent, "phase1", parent.GetUserInput(), true)
	require.NoError(t, loop.ExecuteWithExistedTask(child))

	for _, entry := range invoker.Entries() {
		require.NotEqual(t, aicommon.TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT, entry.Tag)
	}
}
