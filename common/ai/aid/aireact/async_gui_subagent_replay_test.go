package aireact

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type guiChildBuilder func(*reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error)

func (f guiChildBuilder) Build(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
	return f(p)
}

// Strengthen the recorded session: all three owners inspect/list only AFTER
// both child executions exist. Children remain alive until the parent probes
// foreign IDs. The real subagent runtime, parsers, managers and emitters run.
func TestAsyncGUISubagentIsolationReplay(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			fixture := loadGUIReplay(t, "subagent_isolation")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			events := newGUIReplayEvents()
			g := newGUIReplayTools()
			tool := g.tool(t, "bash")
			type child struct {
				manager          *asyncexecmanager.Manager
				id, callID, task string
			}
			var mu sync.Mutex
			children := map[string]child{}
			ready := make(chan struct{})
			checked := make(chan struct{})
			var checkedOnce sync.Once
			markChecked := func() { checkedOnce.Do(func() { close(checked) }) }
			defer markChecked()
			builder := guiChildBuilder(func(p *reactloops.PreparedSubAgent) (*reactloops.ReActLoop, error) {
				key := p.Job.Identifier
				other := "child_a_async_sleep"
				if key == other {
					other = "child_b_async_sleep"
				}
				var loop *reactloops.ReActLoop
				index := 0
				var mine child
				childModel := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": false, "reasoning": "Continue the recorded scenario"}})
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected child request %s", req.GetCallerLabel())
					}
					step := guiReplayStep{}
					defer func() { index++ }()
					manage := func(op, id string) guiReplayStep {
						return guiReplayStep{"manage_async_execution_runtime", aitool.InvokeParams{"op": op, "execute_id": id, "timeout_seconds": 60}}
					}
					switch index {
					case 0:
						step = guiReplayStep{"require_tool", aitool.InvokeParams{"require_tool_payload": "bash"}}
					case 1:
						step = guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": "bash", "directly_call_tool_params": aitool.InvokeParams{"command": "sleep 20; echo CHILD_DONE", "timeout": 60}, "async_directly_call_tool": true, "directly_call_reason": "child background test"}}
					case 2:
						manager, err := loop.GetAsyncExecutionManager()
						if err != nil {
							return nil, err
						}
						entries := manager.List()
						if len(entries) != 1 {
							return nil, fmt.Errorf("child owns %d executions", len(entries))
						}
						mine = child{manager: manager, id: entries[0].ID, callID: entries[0].Metadata.Attributes["call_tool_id"], task: p.Task.GetId()}
						mu.Lock()
						children[key] = mine
						if len(children) == 2 {
							close(ready)
						}
						mu.Unlock()
						if err := guiAwait(ctx, ready); err != nil {
							return nil, err
						}
						step = manage("list", "")
					case 3:
						ownEntries := mine.manager.List()
						if len(ownEntries) != 1 || ownEntries[0].ID != mine.id {
							return nil, fmt.Errorf("sibling leaked into child list after both submissions")
						}
						if !strings.Contains(req.GetPrompt(), mine.id) {
							return nil, fmt.Errorf("child did not receive own list")
						}
						mu.Lock()
						foreign := children[other].id
						mu.Unlock()
						step = manage("inspect", foreign)
					case 4:
						if !strings.Contains(req.GetPrompt(), asyncexecmanager.ErrNotFound.Error()) {
							return nil, fmt.Errorf("child read sibling execution")
						}
						if err := guiAwait(ctx, checked); err != nil {
							return nil, err
						}
						work, err := g.get(ctx, mine.callID)
						if err != nil {
							return nil, err
						}
						if err := guiAwait(ctx, work.started); err != nil {
							return nil, err
						}
						work.release()
						step = manage("wait", mine.id)
					case 5:
						step = manage("inspect", mine.id)
					case 6:
						if !strings.Contains(req.GetPrompt(), "CHILD_DONE") {
							return nil, fmt.Errorf("child output not returned to model")
						}
						step = guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": "CHILD_DONE " + mine.id}}
					default:
						step = guiReplayStep{"finish", aitool.InvokeParams{}}
					}
					return guiRespond(c, req, native, step)
				}
				cfg := p.Invoker.GetConfig().(*aicommon.Config)
				if err := aicommon.WithAICallback(guiMockModel(t, childModel))(cfg); err != nil {
					return nil, err
				}
				opts := reactloops.BasicAICommonConfigOption(cfg)
				opts = append(opts, reactloops.WithFunctionCallMode(native), reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true))
				var err error
				loop, err = reactloops.CreateLoopByName("default", p.Invoker, opts...)
				return loop, err
			})
			var runtime *ReAct
			index := 0
			answer := "GUI_REPLAY_COMPLETE subagent_isolation"
			model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
					return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": false, "reasoning": "Continue the recorded scenario"}})
				}
				if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
					return nil, fmt.Errorf("unexpected parent request %s", req.GetCallerLabel())
				}
				loop := runtime.GetCurrentTask().GetReActLoop().(*reactloops.ReActLoop)
				var step guiReplayStep
				defer func() { index++ }()
				switch index {
				case 0:
					reactloops.WithSubAgentSubmissionPolicy(func(_ []reactloops.SubAgentJob, opts reactloops.SubAgentOptions) (reactloops.SubAgentOptions, error) {
						opts.ElaborateGoals = false
						opts.LoopBuilder = builder
						return opts, nil
					})(loop)
					step = fixture.Steps[0]
				case 1:
					if err := guiAwait(ctx, ready); err != nil {
						return nil, err
					}
					step = fixture.Steps[1]
				case 2, 3:
					mu.Lock()
					key := "child_a_async_sleep"
					if index == 3 {
						key = "child_b_async_sleep"
					}
					foreign := children[key]
					mu.Unlock()
					manager, err := loop.GetAsyncExecutionManager()
					if err != nil {
						return nil, err
					}
					if len(manager.List()) != 0 {
						return nil, fmt.Errorf("child executions leaked to parent")
					}
					if index == 3 && !strings.Contains(req.GetPrompt(), asyncexecmanager.ErrNotFound.Error()) {
						return nil, fmt.Errorf("parent inspected child execution")
					}
					op := "inspect"
					if index == 3 {
						op = "stop"
					}
					step = guiReplayStep{"manage_async_execution_runtime", aitool.InvokeParams{"op": op, "execute_id": foreign.id}}
				case 4:
					if !strings.Contains(req.GetPrompt(), asyncexecmanager.ErrNotFound.Error()) {
						return nil, fmt.Errorf("parent stopped child execution")
					}
					mu.Lock()
					for _, child := range children {
						s, err := child.manager.Get(child.id)
						if err != nil || s.State.IsTerminal() || s.State == asyncexecmanager.StateStopping {
							mu.Unlock()
							return nil, fmt.Errorf("foreign stop affected child")
						}
					}
					mu.Unlock()
					markChecked()
					// Synchronize on the real manager; result timeline events are only
					// published when the parent consumes the observation.
					observation, err := loop.GetSubAgentManager().Wait(ctx, nil, time.Minute, ^uint64(0))
					if err != nil {
						return nil, err
					}
					if observation.TimedOut {
						return nil, fmt.Errorf("children did not finish")
					}
					step = guiReplayStep{"wait_sub_react_agents", aitool.InvokeParams{"timeout_ms": 0}}
				case 5:
					step = guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}}
				default:
					step = guiReplayStep{"finish", aitool.InvokeParams{}}
				}
				return guiRespond(c, req, native, step)
			}
			var err error
			runtime, err = NewTestReAct(
				aicommon.WithContext(ctx),
				aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tool)),
				aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithAgreeYOLO(),
				aicommon.WithWorkdir(t.TempDir()),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithNoOpMemoryTriage(),
				aicommon.WithDisableToolCallerIntervalReview(true),
				aicommon.WithMaxSubAgents(2),
				aicommon.WithEnableDispatchSubReactAgent(true),
				aicommon.WithAICallback(guiMockModel(t, model)),
				aicommon.WithEventHandler(events.record),
				aicommon.WithAIAutoRetry(1),
				aicommon.WithAITransactionAutoRetry(1))
			require.NoError(t, err)
			defer func() {
				g.close()
				cancel()
				markChecked()
				for _, work := range g.all() {
					work.release()
					work.releaseCleanup()
				}
				runtime.WaitLifecycleStopped()
			}()
			runtime.config.SetWorkDir(runtime.config.Workdir)
			require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: fixture.Prompt}))
			guiWaitCompleted(t, ctx, events)
			runtime.Emitter.WaitForStream()
			validateGUIReplayCards(t, events.snapshot(), "subagent_isolation", answer)
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, children, 2)
			var ids, tasks []string
			for _, child := range children {
				ids = append(ids, child.id)
				tasks = append(tasks, child.task)
				s, err := child.manager.Get(child.id)
				require.NoError(t, err)
				require.Equal(t, asyncexecmanager.StateCompleted, s.State)
			}
			require.NotEqual(t, ids[0], ids[1])
			require.NotEqual(t, tasks[0], tasks[1])
		})
	}
}
