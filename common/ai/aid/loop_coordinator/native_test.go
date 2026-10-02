package loop_coordinator_test

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
	"github.com/yaklang/yaklang/common/ai/aid/loop_coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type nativeHost struct{ plan *loop_coordinator.Plan }

type projectedInvoker struct{ *mock.MockInvoker }

func (p *projectedInvoker) AddToTimelineWithPromptProjection(entry, display, prompt string) {
	if cfg, ok := p.GetConfig().(*aicommon.Config); ok {
		cfg.GetTimeline().PushTextWithPromptProjection(cfg.AcquireId(), entry+": "+display, prompt)
	}
}

func (h *nativeHost) Prepare(context.Context, string, string) (*loop_coordinator.Plan, error) {
	return h.plan, nil
}
func (h *nativeHost) Approve(_ context.Context, p *loop_coordinator.Plan) (*loop_coordinator.Plan, error) {
	return p, nil
}
func (h *nativeHost) Execute(context.Context, loop_coordinator.Attempt) (loop_coordinator.Result, error) {
	return loop_coordinator.Result{Summary: "checked", EvidenceIDs: []string{"native.evidence"}}, nil
}
func (h *nativeHost) Changed(loop_coordinator.Snapshot) {}

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

func TestCoordinatorLoopNativeOnlyEvenWithTextModeOption(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host := &nativeHost{plan: &loop_coordinator.Plan{Tasks: []loop_coordinator.Task{{ID: "a", Name: "A", Goal: "check"}}}}
	c := loop_coordinator.New(ctx, host, 1)
	defer c.Close()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls.Add(1)
			s := c.Snapshot()
			name := ""
			args := map[string]any{}
			switch {
			case s.Draft == nil:
				name = "create_plan"
				args = map[string]any{"plan": map[string]any{"name": "plan", "goal": "check", "tasks": []any{map[string]any{"name": "A", "goal": "check", "identifier": "a", "depends_on": []string{}}}}, "plan_document": "document"}
			case s.Approved == nil:
				name = "submit_plan"
				args["plan_version"] = s.DraftVersion
			case s.Attempts["a"].State == loop_coordinator.Pending:
				name = "start_tasks"
			case s.Attempts["a"].State == loop_coordinator.Running:
				name = "wait_tasks"
			case s.Attempts["a"].State == loop_coordinator.AwaitingReview && !s.Attempts["a"].Seen:
				name = "inspect_tasks"
			case s.Attempts["a"].State == loop_coordinator.AwaitingReview:
				name = "review_task"
				args = map[string]any{"task_id": "a", "attempt_id": s.Attempts["a"].ID, "decision": "accept", "reason": "native.evidence verifies the result"}
			default:
				name = "finish"
			}
			if s.Approved != nil {
				require.Contains(t, req.GetPrompt(), "PLAN STATUS")
			}
			require.NotContains(t, req.GetPrompt(), "<|SCHEMA_")
			return nativeResponse(config, req, name, args)
		}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	task := aicommon.NewStatefulTaskBase("native-coordinator", "check", ctx, cfg.GetEmitter(), true)
	loop, err := loop_coordinator.NewLoop(&projectedInvoker{inv}, loop_coordinator.WithController(c), reactloops.WithFunctionCallMode(false), reactloops.WithDisableLoopPerception(true), reactloops.WithDisableIncreaseIteration(true))
	require.NoError(t, err)
	require.NotNil(t, cfg.LiteForgeExecutor, "native loops install their own helper implementation")
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.NoError(t, c.CanFinish())
	require.GreaterOrEqual(t, calls.Load(), int64(6))
}

func TestCoordinatorLoopWorkerNativeResultGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
	loop, err := loop_coordinator.NewWorkerLoop(&projectedInvoker{inv}, reactloops.WithFunctionCallMode(false), reactloops.WithDisableLoopPerception(true))
	require.NoError(t, err)
	require.NotNil(t, cfg.LiteForgeExecutor)
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.Equal(t, int64(3), calls.Load())
	result, ok := loop.GetVariable("coordinator_task_result").(loop_coordinator.Result)
	require.True(t, ok)
	require.Equal(t, "the exact result", result.Summary)
	for _, name := range []string{"request_plan", "request_plan_and_execution", "report_generating", "dispatch_sub_react_agents"} {
		require.NotContains(t, strings.Join(loop.GetAllActionNames(), ","), name)
	}
}

func TestCoordinatorLoopRejectsJSONResponseActions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := loop_coordinator.New(ctx, &nativeHost{plan: &loop_coordinator.Plan{Tasks: []loop_coordinator.Task{{ID: "a", Name: "A", Goal: "check"}}}}, 1)
	defer c.Close()
	cfg := aicommon.NewConfig(ctx, aicommon.WithAITransactionAutoRetry(1), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		wire.FinishReasonCallback("stop", nil)
		resp := c.NewAIResponse()
		resp.EmitOutputStream(strings.NewReader(`{"@action":"create_plan","plan":{"name":"malicious fallback"}}`))
		resp.Close()
		return resp, nil
	}))
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(cfg)
	loop, err := loop_coordinator.NewLoop(&projectedInvoker{inv}, loop_coordinator.WithController(c))
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("reject-json", "check", ctx, cfg.GetEmitter(), true)
	require.Error(t, loop.ExecuteWithExistedTask(task))
	require.Nil(t, c.Snapshot().Draft)
}

func TestCoordinatorLoopAuxiliaryUsesNativeOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int64
	cfg := aicommon.NewConfig(ctx, aicommon.WithAITransactionAutoRetry(1), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
	for _, option := range loop_coordinator.NativeOptions() {
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
	c, err := loop_coordinator.NewSession(ctx, "Prepare an approved plan without executing it", aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAgreeYOLO(), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		lastPrompt = req.GetPrompt()
		switch calls.Add(1) {
		case 1:
			return nativeResponse(c, req, "create_plan", map[string]any{"plan": map[string]any{"name": "Planning only", "goal": "Prepare execution", "tasks": []any{map[string]any{"name": "Check", "goal": "Check after approval", "identifier": "check", "depends_on": []string{}}}}, "plan_document": "# Approved document"})
		case 2:
			return nativeResponse(c, req, "submit_plan", map[string]any{"plan_version": 1})
		default:
			return nativeResponse(c, req, "finish", map[string]any{})
		}
	}))
	require.NoError(t, err)
	require.NoError(t, c.RunPlanOnly())
	require.Equal(t, int64(3), calls.Load())
	require.Contains(t, lastPrompt, "planning-only")
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
			a, err := loop_coordinator.NativeRiskReview(ctx, cfg, ep)
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
	c := loop_coordinator.New(ctx, &nativeHost{}, 1)
	defer c.Close()
	inv := mock.NewMockInvoker(ctx)
	inv.SetConfig(aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir())))
	loop, err := loop_coordinator.NewLoop(&projectedInvoker{inv}, loop_coordinator.WithController(c))
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		params aitool.InvokeParams
	}{
		{"cancel_tasks", aitool.InvokeParams{"task_ids": nil, "reason": "stop"}},
		{"start_tasks", aitool.InvokeParams{"task_ids": "all"}},
		{"submit_plan", aitool.InvokeParams{"plan_version": "1"}},
		{"review_task", aitool.InvokeParams{"task_id": "a", "attempt_id": 1, "decision": "accept"}},
		{"create_plan", aitool.InvokeParams{"plan": map[string]any{"name": "test", "goal": "check", "tasks": []any{map[string]any{"name": "step", "goal": "check"}}}, "plan_document": "document"}},
	} {
		handler, err := loop.GetActionHandler(test.name)
		require.NoError(t, err)
		require.Error(t, handler.ActionVerifier(loop, aicommon.NewSimpleAction(test.name, test.params)), test.name)
	}
	handler, err := loop.GetActionHandler("start_tasks")
	require.NoError(t, err)
	require.NoError(t, handler.ActionVerifier(loop, aicommon.NewSimpleAction("start_tasks", aitool.InvokeParams{})))
	require.Nil(t, c.Snapshot().Draft)
}

func TestCoordinatorLoopKeywordSearchUsesNativeAuxiliary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := loop_coordinator.NewSession(ctx, "Find a reading tool", aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
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
