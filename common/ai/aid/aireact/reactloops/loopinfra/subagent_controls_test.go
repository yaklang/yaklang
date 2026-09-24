package loopinfra

import (
	"bytes"
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
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBackgroundDispatchActions_RejectMalformedSelectors(t *testing.T) {
	inv := mock.NewMockInvoker(context.Background())
	loop := reactloops.NewMinimalReActLoop(inv.GetConfig(), inv)
	defer loop.Release()
	parse := func(payload string) *aicommon.Action {
		action, err := aicommon.ExtractAction(payload, reactloops.SubAgentCancelAction, reactloops.SubAgentWaitAction)
		require.NoError(t, err)
		return action
	}
	for _, selector := range []string{`"one-job"`, `null`, `[1]`, `[""]`, `{}`} {
		action := parse(`{"@action":"cancel_sub_react_agents","job_ids":` + selector + `}`)
		require.Error(t, loopAction_CancelSubAgents.ActionVerifier(loop, action), selector)
	}
	for _, timeout := range []string{`null`, `"30"`, `1.5`, `-1`, `600001`} {
		action := parse(`{"@action":"wait_sub_react_agents","timeout_ms":` + timeout + `}`)
		require.Error(t, loopAction_WaitSubAgents.ActionVerifier(loop, action), timeout)
	}
	for _, params := range []string{``, `,"job_ids":[]`, `,"job_ids":["one-job"],"timeout_ms":0`, `,"timeout_ms":60001`, `,"timeout_ms":600000`} {
		action := parse(`{"@action":"wait_sub_react_agents"` + params + `}`)
		require.NoError(t, loopAction_WaitSubAgents.ActionVerifier(loop, action))
	}
}

func TestBackgroundDispatchActions_ActualLoop(t *testing.T) {
	for _, functionCall := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", functionCall), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			release := make(chan struct{})
			started := make(chan struct{}, 1)
			var once sync.Once
			releaseChild := func() { once.Do(func() { close(release) }) }
			defer releaseChild()
			var requests atomic.Int32
			capture := &capturedEvents{}
			var loop *reactloops.ReActLoop
			childName := fmt.Sprintf("background-action-child-%v", functionCall)
			require.NoError(t, reactloops.RegisterLoopFactory(childName, func(inv aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
				child := reactloops.NewMinimalReActLoop(inv.GetConfig(), inv)
				for _, opt := range opts {
					opt(child)
				}
				reactloops.WithInitTask(func(_ *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
					started <- struct{}{}
					select {
					case <-release:
						task.SetResult("ACTUAL_CHILD_EVIDENCE")
					case <-task.GetContext().Done():
					}
					op.Done()
				})(child)
				return child, nil
			}))
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithEventHandler(capture.appendEvent),
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableMultiAgentMode(true), aicommon.WithEnableFunctionCallMode(functionCall),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					body := ""
					if strings.Contains(req.GetPrompt(), "You are preparing a task brief") {
						body = `{"@action":"object","goal":"inspect payment log","result_contract":"return evidence"}`
					} else {
						step := requests.Add(1)
						require.False(t, req.IsToolCallArgumentsStreamEnabled())
						switch step {
						case 1:
							body = fmt.Sprintf(`{"@action":"dispatch_sub_react_agents","dispatches":[{"goal":"inspect payment","loop_name":%q}]}`, childName)
						case 2:
							body = `{"@action":"read_deployment"}`
						case 3:
							body = `{"@action":"wait_sub_react_agents","timeout_ms":1}`
						case 4:
							require.Contains(t, req.GetPrompt(), "observation_timeout")
							// Waiting must be visible before the next model request; its
							// protocol JSON must not be rendered as human-facing text.
							loop.GetEmitter().WaitForStream()
							var report, waitText strings.Builder
							for _, event := range capture.byType(schema.EVENT_TYPE_STREAM) {
								text := string(event.Content) + string(event.StreamDelta)
								if event.NodeId == loopInfraNodeSubReactReport {
									report.WriteString(text)
								}
								if event.NodeId == "sub_react_agents_wait" {
									waitText.WriteString(text)
								}
							}
							require.Contains(t, report.String(), "已派发 1 个子任务")
							require.NotContains(t, report.String(), `"job_id"`)
							require.Contains(t, waitText.String(), "正在等待 1 个子任务")
							require.Contains(t, waitText.String(), "本次等待已到时，子任务继续执行")
							waiting, finished := false, false
							for _, event := range capture.byType(schema.EVENT_TYPE_STRUCTURED) {
								if event.NodeId != "status" {
									continue
								}
								var payload aicommon.StatusPayload
								require.NoError(t, json.Unmarshal(event.Content, &payload))
								if payload.Code == "subagent.waiting" {
									waiting = true
									require.Equal(t, aicommon.StatusStateWaiting, payload.State)
								}
							}
							for _, event := range capture.byType(schema.EVENT_TYPE_STRUCTURED) {
								if event.NodeId == "stream-finished" && strings.Contains(string(event.Content), `"node_id":"sub_react_agents_wait"`) {
									finished = true
								}
							}
							require.True(t, waiting, "waiting is not model reasoning")
							require.True(t, finished, "the visible wait stream must end")
							snapshots, err := loop.GetSubAgentManager().Inspect(nil)
							require.NoError(t, err)
							require.Len(t, snapshots, 1)
							require.NotContains(t, []string{"completed", "cancelled", "failed"}, snapshots[0].State)
							releaseChild()
							require.Eventually(t, func() bool {
								items, _ := loop.GetSubAgentManager().Inspect(nil)
								return len(items) == 1 && items[0].ResultRevision > 0
							}, time.Second, time.Millisecond)
							body = `{"@action":"finish"}` // result was not in this request; must be refused
						case 5:
							require.Contains(t, req.GetPrompt(), "ACTUAL_CHILD_EVIDENCE")
							body = `{"@action":"finish"}`
						default:
							t.Errorf("unexpected parent call %d", step)
							return nil, context.Canceled
						}
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
			aicommon.AIRuntimeInvokerGetter = func(ctx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
				child := mock.NewMockInvoker(ctx)
				child.SetConfig(aicommon.NewConfig(ctx, opts...))
				return child, nil
			}
			var err error
			loop, err = reactloops.NewReActLoop("background-action-parent", inv,
				reactloops.WithFunctionCallMode(functionCall),
				reactloops.WithReactiveDataBuilder(func(_ *reactloops.ReActLoop, feedback *bytes.Buffer, _ string) (string, error) {
					return feedback.String(), nil
				}),
				reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
				reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
				reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true),
				reactloops.WithRegisterLoopAction("read_deployment", "read independent deployment config", nil, nil,
					func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
						select {
						case <-started:
						case <-ctx.Done():
							t.Error("child not started")
						}
						select {
						case <-release:
							t.Error("parent local work ran after child completion")
						default:
						}
						op.Continue()
					}))
			require.NoError(t, err)
			require.NoError(t, loop.Execute("actual-background-parent", ctx, "delegate payment analysis and read deployment configuration"))
			require.EqualValues(t, 5, requests.Load())
		})
	}
}

func TestSubAgentObservationSummaryDistinguishesTerminalStates(t *testing.T) {
	jobs := []reactloops.SubAgentSnapshot{
		{State: "completed"}, {State: "failed"}, {State: "cancelled"},
		{State: "timed_out"}, {State: "cancelling"}, {State: "running", LatestOutput: "answer already emitted"},
	}
	zh, en, settled := subAgentObservationSummary(jobs)
	require.Equal(t, 4, settled)
	require.Contains(t, zh, "1 个已完成")
	require.Contains(t, zh, "1 个执行中")
	require.Contains(t, zh, "1 个取消中")
	require.Contains(t, zh, "1 个执行超时")
	require.Contains(t, en, "1 failed")
	require.NotContains(t, zh, "answer already emitted")
}
