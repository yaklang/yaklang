package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type waitingHost struct {
	*nativeHost
	release chan struct{}
}

func (h *waitingHost) Execute(ctx context.Context, a coordinator.Attempt) (coordinator.Result, error) {
	select {
	case <-ctx.Done():
		return coordinator.Result{}, ctx.Err()
	case <-h.release:
		return coordinator.Result{Summary: a.Task.Name + " verified", EvidenceIDs: []string{"checked." + a.Task.ID}}, nil
	}
}

// Exercise the actual prompt/action/post-iteration loop. A CURRENT TODO for
// future review must not poll the model, and a final wait in a native batch must
// not hide completed reviews. Both protocols retain the report/TODO finish gate.
func TestCoordinatorWaitConvergenceWithOpenTodo(t *testing.T) {
	for _, mode := range []string{"text", "native_wait_last", "native_wait_first"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			host := &waitingHost{nativeHost: &nativeHost{plan: &coordinator.Plan{Tasks: []coordinator.Task{{ID: "a", Name: "A", Goal: "check"}, {ID: "b", Name: "B", Goal: "check"}}}}, release: make(chan struct{})}
			c := coordinator.New(ctx, host, 2)
			defer c.Close()
			native := mode != "text"
			var calls atomic.Int64
			ready := make(chan struct{})
			closedTodo := false
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					step := calls.Add(1)
					if step > 12 {
						return nil, fmt.Errorf("coordinator did not converge: %d requests", step)
					}
					s := c.Snapshot()
					switch {
					case s.Plan == nil:
						return protocolResponse(config, req, native, "create_plan", map[string]any{"plan": map[string]any{"name": "test", "goal": "check", "tasks": []any{map[string]any{"name": "A", "goal": "check", "identifier": "a"}, map[string]any{"name": "B", "goal": "check", "identifier": "b"}}}, "plan_document": "A/B independent"})
					case s.Phase == coordinator.PhasePlan:
						close(ready)
						return protocolResponse(config, req, native, "submit_plan", map[string]any{})
					}
					for _, id := range []string{"a", "b"} {
						a := s.Attempts[id]
						if a.State != coordinator.AwaitingReview {
							continue
						}
						args := map[string]any{"identifier": "review_result", "task_id": id, "attempt_id": a.ID, "decision": "accept", "reason": "checked." + id + " verifies the result"}
						if !native {
							return protocolResponse(config, req, false, "review_task", args)
						}
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						data, _ := json.Marshal(args)
						review := &aispec.ToolCall{ID: fmt.Sprintf("review_%d", step), Type: "function", Function: aispec.FuncReturn{Name: "review_task", Arguments: string(data)}}
						wait := &aispec.ToolCall{ID: fmt.Sprintf("wait_%d", step), Type: "function", Function: aispec.FuncReturn{Name: "wait_messages", Arguments: `{"identifier":"wait_for_results"}`}}
						batch := []*aispec.ToolCall{review, wait}
						if mode == "native_wait_first" {
							batch = []*aispec.ToolCall{wait, review}
						}
						wire.ToolCallCallback(batch)
						wire.FinishReasonCallback("tool_calls", nil)
						resp := config.NewAIResponse()
						resp.Close()
						return resp, nil
					}
					if s.Attempts["a"].State != coordinator.Accepted || s.Attempts["b"].State != coordinator.Accepted {
						return nil, fmt.Errorf("unexpected model wake with no reviewable work")
					}
					if !closedTodo {
						closedTodo = true
						args := map[string]any{"todo_delta": map[string]any{"close": []any{map[string]any{"id": "review", "outcome": "resolved", "reason": "A/B checked", "refs": []string{"checked.a", "checked.b"}}}}}
						if native {
							return protocolResponse(config, req, true, "adjust_todolist", args)
						}
						args["evidence_id"], args["evidence_content"] = "reviews", "A/B results were reviewed and accepted"
						return protocolResponse(config, req, false, "save_evidence", args)
					}
					return reportResponse(config, req, native, s.Report.Path != "")
				}))
			inv := mock.NewMockInvoker(ctx)
			inv.SetConfig(cfg)
			loop, err := coordinator.NewLoop(&projectedInvoker{inv}, coordinator.WithController(c), reactloops.WithFunctionCallMode(native), reactloops.WithDisableLoopPerception(true))
			require.NoError(t, err)
			c.EnableExecution(false, 0)
			task := aicommon.NewStatefulTaskBase("wait-test", "check", ctx, cfg.GetEmitter(), true)
			loop.SetCurrentTask(task)
			current := "review"
			results := cfg.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{Add: []aicommon.TodoAdd{{ID: "review", Text: "等 A/B 返回后审核结果"}}, Current: &current, CurrentSet: true})
			require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
			done := make(chan error, 1)
			go func() { done <- loop.ExecuteWithExistedTask(task) }()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("did not reach automatic wait")
			}
			time.Sleep(80 * time.Millisecond)
			require.Equal(t, int64(2), calls.Load(), "open CURRENT TODO must not cause empty model calls")
			require.Len(t, aicommon.GetBlockingVerificationTodoItems(cfg, task), 1)
			close(host.release)
			require.NoError(t, <-done)
			require.True(t, c.Snapshot().Finished)
			require.Empty(t, c.Snapshot().Inbox)
			require.LessOrEqual(t, calls.Load(), int64(9))
		})
	}
}
