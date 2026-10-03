package coordinator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type nativeHost struct{ plan *coordinator.Plan }

type projectedInvoker struct{ *mock.MockInvoker }

func (p *projectedInvoker) AddToTimelineWithPromptProjection(entry, display, prompt string) {
	if cfg, ok := p.GetConfig().(*aicommon.Config); ok {
		cfg.GetTimeline().PushTextWithPromptProjection(cfg.AcquireId(), entry+": "+display, prompt)
	}
}

func (h *nativeHost) Prepare(_ context.Context, _ string, document string) (*coordinator.Plan, error) {
	copy := *h.plan
	copy.Document = document
	if len(copy.Tree) == 0 {
		root := &coordinator.PlanNode{TaskID: "root", Name: "test", Goal: "check", Identifier: "root"}
		for _, task := range copy.Tasks {
			root.Subtasks = append(root.Subtasks, &coordinator.PlanNode{TaskID: task.ID, Name: task.Name, Goal: task.Goal, Identifier: task.ID, DependsOn: task.DependsOn})
		}
		raw, _ := json.Marshal(root)
		return coordinator.ParsePlan(string(raw), document, nil)
	}
	return &copy, nil
}
func (h *nativeHost) Approve(_ context.Context, p *coordinator.Plan) (*coordinator.Plan, error) {
	return p, nil
}
func (h *nativeHost) Execute(context.Context, coordinator.Attempt) (coordinator.Result, error) {
	return coordinator.Result{Summary: "checked", EvidenceIDs: []string{"native.evidence"}}, nil
}
func (h *nativeHost) Changed(coordinator.Snapshot) {}

func nativeResponse(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, args any) (*aicommon.AIResponse, error) {
	cfg := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	if cfg.ToolCallCallback == nil || cfg.FinishReasonCallback == nil {
		return nil, fmt.Errorf("request did not use native function calls: %s", req.GetCallerLabel())
	}
	data, _ := json.Marshal(args)
	cfg.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("call_%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
	cfg.FinishReasonCallback("tool_calls", nil)
	resp := c.NewAIResponse()
	resp.Close()
	return resp, nil
}

func protocolResponse(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, native bool, name string, args map[string]any) (*aicommon.AIResponse, error) {
	args["identifier"] = name
	if native {
		return nativeResponse(c, req, name, args)
	}
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	if len(wire.Tools) != 0 || wire.ToolCallCallback != nil {
		return nil, fmt.Errorf("text action request unexpectedly advertised native calls")
	}
	args["@action"] = name
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp := c.NewAIResponse()
	resp.EmitOutputStream(strings.NewReader(string(data)))
	resp.Close()
	return resp, nil
}

func TestCoordinatorLoopNativeProtocol(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host := &nativeHost{plan: &coordinator.Plan{Tasks: []coordinator.Task{{ID: "a", Name: "A", Goal: "check"}}}}
	c := coordinator.New(ctx, host, 1)
	defer c.Close()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls.Add(1)
			s := c.Snapshot()
			name := ""
			args := map[string]any{}
			switch {
			case s.Plan == nil:
				name = "create_plan"
				args = map[string]any{"plan": map[string]any{"name": "plan", "goal": "check", "tasks": []any{map[string]any{"name": "A", "goal": "check", "identifier": "a", "depends_on": []string{}}}}, "plan_document": "document"}
			case s.Phase == coordinator.PhasePlan:
				name = "submit_plan"
			case s.Attempts["a"].State == coordinator.Pending:
				name = "wait_messages"
			case s.Attempts["a"].State == coordinator.Running:
				name = "wait_messages"
			case s.Attempts["a"].State == coordinator.AwaitingReview:
				name = "review_task"
				args = map[string]any{"task_id": "a", "attempt_id": s.Attempts["a"].ID, "decision": "accept", "reason": "native.evidence verifies the result"}
			default:
				return reportResponse(config, req, true, s.Report.Path != "")
			}
			if s.Phase == coordinator.PhaseExec {
				require.Contains(t, req.GetPrompt(), "PLAN STATUS")
			}
			require.NotContains(t, req.GetPrompt(), "<|SCHEMA_")
			return nativeResponse(config, req, name, args)
		}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	task := aicommon.NewStatefulTaskBase("native-coordinator", "check", ctx, cfg.GetEmitter(), true)
	loop, err := coordinator.NewLoop(&projectedInvoker{inv}, coordinator.WithController(c), reactloops.WithFunctionCallMode(true), reactloops.WithDisableLoopPerception(true), reactloops.WithDisableIncreaseIteration(true))
	require.NoError(t, err)
	require.NotNil(t, cfg.LiteForgeExecutor, "native loops install their own helper implementation")
	c.EnableExecution(false, 0)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.True(t, c.Snapshot().Finished)
	require.GreaterOrEqual(t, calls.Load(), int64(5))
}

func TestCoordinatorLoopWorkerNativeResultGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		step := calls.Add(1)
		if step == 1 {
			return nativeResponse(c, req, "finish", map[string]any{})
		}
		if step == 2 {
			return nativeResponse(c, req, "submit_task_result", map[string]any{"summary": "the exact result", "evidence_ids": []string{"e1"}})
		}
		return nativeResponse(c, req, "finish", map[string]any{})
	}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	task := aicommon.NewStatefulTaskBase("native-worker", "verify", ctx, cfg.GetEmitter(), true)
	loop, err := coordinator.NewWorkerLoop(&projectedInvoker{inv}, reactloops.WithFunctionCallMode(true), reactloops.WithDisableLoopPerception(true))
	require.NoError(t, err)
	require.NotNil(t, cfg.LiteForgeExecutor)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.Equal(t, int64(3), calls.Load())
	result, ok := loop.GetVariable("coordinator_task_result").(coordinator.Result)
	require.True(t, ok)
	require.Equal(t, "the exact result", result.Summary)
	for _, name := range []string{"request_plan", "request_plan_and_execution", "report_generating", "dispatch_sub_react_agents"} {
		require.NotContains(t, strings.Join(loop.GetAllActionNames(), ","), name)
	}
}

func TestCoordinatorLoopWorkerCompatibleArguments(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("function_call_%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var calls atomic.Int64
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(native), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				if calls.Add(1) == 1 {
					args := map[string]any{"summary": "verified result", "evidence_ids": []string{"e1"}, "future_business_context": map[string]any{"version": 2}}
					if !native {
						args["human_readable_thought"] = "提交真实结果"
						args["todo_delta"] = map[string]any{}
					}
					return protocolResponse(c, req, native, "submit_task_result", args)
				}
				return protocolResponse(c, req, native, "finish", map[string]any{})
			}))
			inv := mock.NewMockInvoker(ctx)
			inv.SetConfig(cfg)
			loop, err := coordinator.NewWorkerLoop(&projectedInvoker{inv}, reactloops.WithFunctionCallMode(native), reactloops.WithDisableLoopPerception(true))
			require.NoError(t, err)
			task := aicommon.NewStatefulTaskBase("compatible-worker", "check", ctx, cfg.GetEmitter(), true)
			require.NoError(t, loop.ExecuteWithExistedTask(task))
			require.Equal(t, int64(2), calls.Load(), "extra fields must not trigger transaction retries")
			result, ok := loop.GetVariable("coordinator_task_result").(coordinator.Result)
			require.True(t, ok)
			require.Equal(t, "verified result", result.Summary)
		})
	}
}

func TestCoordinatorLoopRejectsJSONResponseActions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := coordinator.New(ctx, &nativeHost{plan: &coordinator.Plan{Tasks: []coordinator.Task{{ID: "a", Name: "A", Goal: "check"}}}}, 1)
	defer c.Close()
	cfg := aicommon.NewConfig(ctx, aicommon.WithAITransactionAutoRetry(1), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		wire.FinishReasonCallback("stop", nil)
		resp := c.NewAIResponse()
		resp.EmitOutputStream(strings.NewReader(`{"@action":"create_plan","plan":{"name":"malicious fallback"}}`))
		resp.Close()
		return resp, nil
	}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	loop, err := coordinator.NewLoop(&projectedInvoker{inv}, coordinator.WithController(c))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("reject-json", "check", ctx, cfg.GetEmitter(), true)
	require.Error(t, loop.ExecuteWithExistedTask(task))
	require.Nil(t, c.Snapshot().Plan)
}

func TestCoordinatorLoopAuxiliaryUsesNativeOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithAITransactionAutoRetry(1), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		require.Len(t, wire.Tools, 1)
		require.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": wire.Tools[0].Function.Name}}, wire.ToolChoice)
		require.NotContains(t, req.GetPrompt(), "输出 JSON")
		messages := aiprojection.Project(aiprojection.ProjectionInput{Prompt: req.GetPrompt()}).Messages
		require.NotEmpty(t, messages)
		require.Equal(t, "system", messages[0].Role)
		require.Contains(t, fmt.Sprint(messages[0].Content), "advertised native function exactly once")
		calls.Add(1)
		return nativeResponse(c, req, wire.Tools[0].Function.Name, map[string]any{"summary": "native infrastructure summary"})
	}), aicommon.WithEnableFunctionCallMode(true))
	for _, option := range coordinator.NativeOptions() {
		require.NoError(t, option(cfg))
	}
	var summary string
	var outputErr error
	cfg.ScheduleAuxiliaryTask(ctx, "coordinator-native-summary", func() string { return "Summarize these verified facts." }, func(a *aicommon.Action) { summary = a.GetString("summary") }, aicommon.WithAuxiliaryOutputs(aitool.WithStringParam("summary", aitool.WithParam_Required())), aicommon.WithAuxiliaryOnError(func(err error) { outputErr = err }))
	require.NoError(t, outputErr)
	require.Equal(t, "native infrastructure summary", summary)
	require.Equal(t, int64(1), calls.Load())
}

func TestCoordinatorLoopPlanOnlyUsesSameNativeLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls atomic.Int64
	var lastPrompt string
	c, err := coordinator.NewSession(ctx, "Prepare an approved plan without executing it", aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAgreeYOLO(), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		lastPrompt = req.GetPrompt()
		switch calls.Add(1) {
		case 1:
			return nativeResponse(c, req, "create_plan", map[string]any{"plan": map[string]any{"name": "Planning only", "goal": "Prepare execution", "tasks": []any{map[string]any{"name": "Check", "goal": "Check after approval", "identifier": "check", "depends_on": []string{}}}}, "plan_document": "# Approved document"})
		case 2:
			return nativeResponse(c, req, "submit_plan", map[string]any{})
		default:
			return nativeResponse(c, req, "finish", map[string]any{})
		}
	}))
	require.NoError(t, err)
	require.NoError(t, c.RunPlanOnly())
	require.Equal(t, int64(3), calls.Load())
	require.Contains(t, lastPrompt, "本次只负责规划")
	require.Contains(t, lastPrompt, "pending; attempt=0")
}

func TestCoordinatorLoopRiskReviewNativeRequiredFields(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg := aicommon.NewConfig(ctx, aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				args := map[string]any{"reason": "Read-only authorized exploration"}
				if valid {
					args["risk_score"] = 0.1
				}
				return nativeResponse(c, req, "review_risk", args)
			}))
			ep := cfg.Epm.CreateEndpoint()
			a, err := coordinator.NativeRiskReview(ctx, cfg, ep)
			if valid {
				require.NoError(t, err)
				require.Equal(t, 0.1, a.GetFloat("risk_score"))
			} else {
				require.Error(t, err)
				require.Nil(t, a)
			}
		})
	}
}

func TestCoordinatorLoopRejectsMalformedFunctionArguments(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := coordinator.New(ctx, &nativeHost{}, 1)
	defer c.Close()
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir())))
	loop, err := coordinator.NewLoop(&projectedInvoker{inv}, coordinator.WithController(c))
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		params aitool.InvokeParams
	}{
		{"cancel_tasks", aitool.InvokeParams{"task_ids": nil, "reason": "stop"}},
		{"wait_messages", aitool.InvokeParams{"timeout_seconds": "all"}},
		{"review_task", aitool.InvokeParams{"task_id": "a", "attempt_id": 1, "decision": "accept"}},
		{"create_plan", aitool.InvokeParams{"plan": map[string]any{"name": "test", "goal": "check", "tasks": []any{map[string]any{"name": "step", "goal": "check"}}}, "plan_document": "document"}},
	} {
		handler, err := loop.GetActionHandler(test.name)
		require.NoError(t, err)
		require.Error(t, handler.ActionVerifier(loop, aicommon.NewSimpleAction(test.name, test.params)), test.name)
	}
	handler, err := loop.GetActionHandler("wait_messages")
	require.NoError(t, err)
	require.NoError(t, handler.ActionVerifier(loop, aicommon.NewSimpleAction("wait_messages", aitool.InvokeParams{})))
	require.Nil(t, c.Snapshot().Plan)
}

func TestCoordinatorLoopKeywordSearchUsesNativeAuxiliary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := coordinator.NewSession(ctx, "Find a reading tool", aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(true), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		require.NotContains(t, req.GetPrompt(), "<|SCHEMA_")
		return nativeResponse(c, req, "keyword_search", map[string]any{"matches": []any{map[string]any{"tool": "read_file", "matched_keywords": []string{"read"}}}})
	}))
	require.NoError(t, err)
	catalog := omap.NewOrderedMap[string, []string](nil)
	catalog.Set("read_file", []string{"read", "file"})
	results, err := c.HandleSearch("read source", catalog)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "read_file", results[0].Key)
}
