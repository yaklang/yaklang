package aireact

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestReAct_FirstOverflowShrinksPrefixAndLearnsAfterSuccess(t *testing.T) {
	model, newest := uuid.NewString(), uuid.NewString()
	respond := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest, action string, params map[string]any) *aicommon.AIResponse {
		rsp := compressionActionResponse(t, cfg, req, action, params)
		rsp.SetModelInfo("test", model)
		if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
			rsp.SetModelInfo("other-provider", "quality-compressor")
		}
		return rsp
	}
	failedTokens, compressionCalls := 0, 0
	var react *ReAct
	react, err := NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
				return respond(cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
			}
			if failedTokens == 0 {
				failedTokens = aicommon.MeasureTokens(req.GetPrompt())
				return nil, &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "test", Model: model, MaxContextTokens: 128000}}
			}
			if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
				compressionCalls++
				require.Equal(t, "default", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model).Source, "failed requests must not save a capacity")
				require.Less(t, aicommon.MeasureTokens(req.GetPrompt()), failedTokens)
				require.NotContains(t, req.GetPrompt(), newest, "compress only the older prefix")
				return respond(cfg, req, aicommon.ContextCompressionAction, map[string]any{
					"summary": "Earlier investigation has been summarized.", "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{},
				}), nil
			}
			require.Equal(t, "default", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model).Source,
				"another model's successful compression must not confirm the failed model")
			require.Contains(t, req.GetPrompt(), newest, "keep history outside the compression prefix")
			return respond(cfg, req, "finish", map[string]any{}), nil
		}))
	require.NoError(t, err)
	react.config.UpdateAIModelInfo("test", model)
	for i := 0; i < 6; i++ {
		marker := uuid.NewString()
		if i == 5 {
			marker = newest
		}
		react.config.Timeline.PushText(react.config.AcquireId(), "%s %s", marker, strings.Repeat(uuid.NewString()+" ", 500))
	}
	loop, err := reactloops.NewReActLoop("first-overflow", react,
		reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute(uuid.NewString(), ctx, "Continue the investigation."))
	require.Equal(t, 1, compressionCalls)
	require.Equal(t, yakit.AIModelContext{ContextTokens: 128000, ThresholdTokens: 102400, Source: "learned"}, yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model))
}

func TestReAct_OverflowCompressionUsesLastFittingBoundary(t *testing.T) {
	kept, dropped := uuid.NewString(), uuid.NewString()
	var prompts []string
	var react *ReAct
	react, err := NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
				return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
			}
			prompt := req.GetPrompt()
			prompts = append(prompts, prompt)
			switch len(prompts) {
			case 1:
				require.Contains(t, prompt, kept)
				require.NotContains(t, prompt, dropped)
				return compressionActionResponse(t, cfg, req, "probe", map[string]any{}), nil
			case 2:
				require.Contains(t, prompt, dropped)
				return nil, &aispec.PromptOverflowError{}
			case 3:
				require.Equal(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel())
				require.Contains(t, prompt, kept)
				require.NotContains(t, prompt, dropped)
				return compressionActionResponse(t, cfg, req, aicommon.ContextCompressionAction, map[string]any{
					"summary": uuid.NewString(), "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{},
				}), nil
			default:
				require.Contains(t, prompt, dropped, "history after the fitting boundary stays for the next normal turn")
				return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
			}
		}))
	require.NoError(t, err)
	react.config.Timeline.PushText(react.config.AcquireId(), "%s", kept)
	loop, err := reactloops.NewReActLoop("overflow-fitting-boundary", react,
		reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true),
		reactloops.WithMaxIterations(4),
		reactloops.WithRegisterLoopAction("probe", "record one later timeline item", nil, nil,
			func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				react.config.Timeline.PushText(react.config.AcquireId(), "%s", dropped)
				op.Continue()
			}))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("overflow-fitting-"+uuid.NewString(), ctx, uuid.NewString()))
	require.GreaterOrEqual(t, len(prompts), 4)
	require.Contains(t, aicommon.RenderTimelineFrozenOpen(react.config.Timeline).PromptText(), dropped)
}

// Even a provider that rejects every rebuilt prompt must not keep an AI task
// alive forever or poison the model's durable budget with an unverified guess.
func TestReAct_PermanentOverflowIsBoundedAndNeverLearns(t *testing.T) {
	model := uuid.NewString()
	calls := 0
	react, err := NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls++
			return nil, &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "test", Model: model, MaxContextTokens: 128000}}
		}))
	require.NoError(t, err)
	react.config.UpdateAIModelInfo("test", model)
	loop, err := reactloops.NewReActLoop("permanent-overflow", react,
		reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(100))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = loop.Execute(uuid.NewString(), ctx, "Continue working.")
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "retry budget, not timeout, must terminate recovery")
	budget := react.config.GetPromptOverflowRetryMax() + 1
	require.LessOrEqual(t, calls, budget*budget)
	require.Greater(t, calls, 1)
	require.Equal(t, "default", yakit.GetAIModelContext(consts.GetGormProfileDatabase(), "test", model).Source)
}

func TestReAct_CompressionSuccessCannotResetOverflowBudget(t *testing.T) {
	model := uuid.NewString()
	normalCalls, compressionCalls := 0, 0
	react, err := NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
				compressionCalls++
				rsp := compressionActionResponse(t, cfg, req, aicommon.ContextCompressionAction, map[string]any{
					"summary": "The same task still needs investigation.", "retain_timeline": "1-1000000", "retire_skill_names": []string{}, "retire_evidence_ids": []string{},
				})
				rsp.SetModelInfo("test", model)
				return rsp, nil
			}
			normalCalls++
			return nil, &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "test", Model: model, MaxContextTokens: 128000}}
		}))
	require.NoError(t, err)
	react.config.UpdateAIModelInfo("test", model)
	for i := 0; i < 6; i++ {
		react.config.Timeline.PushText(react.config.AcquireId(), "%s", strings.Repeat("old investigation ", 500))
	}
	loop, err := reactloops.NewReActLoop("overflow-with-successful-maintenance", react,
		reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(100))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = loop.Execute(uuid.NewString(), ctx, "Continue investigating.")
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "successful maintenance must not allow endless failing business requests")
	budget := react.config.GetPromptOverflowRetryMax() + 1
	require.LessOrEqual(t, normalCalls, budget*budget)
	require.Greater(t, compressionCalls, 1, "exercise repeated successful maintenance without normal request progress")
}

func TestReAct_DisabledAutoAdjustStopsOnFirstOverflow(t *testing.T) {
	for _, withIdentity := range []bool{true, false} {
		t.Run(fmt.Sprint("response_identity=", withIdentity), func(t *testing.T) {
			model := uuid.NewString()
			disabled := true
			db := consts.GetGormProfileDatabase()
			require.NoError(t, yakit.SetAIModelContextSettings(db, "test", model, 500*1024, &disabled))
			before := yakit.GetAIModelContext(db, "test", model)
			calls, compressionCalls := 0, 0
			react, err := NewTestReAct(
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithAIAutoRetry(3), aicommon.WithAITransactionAutoRetry(3),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls++
					if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
						compressionCalls++
					}
					evidence := aispec.PromptOverflowEvidence{MaxContextTokens: 128000}
					if withIdentity {
						evidence.Provider, evidence.Model = "test", model
					}
					return nil, &aispec.PromptOverflowError{PromptOverflowEvidence: evidence}
				}))
			require.NoError(t, err)
			react.config.UpdateAIModelInfo("test", model)
			loop, err := reactloops.NewReActLoop("manual-overflow", react, reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(100))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err = loop.Execute(uuid.NewString(), ctx, "Continue working.")
			require.ErrorContains(t, err, "自动调整已关闭")
			require.True(t, aicommon.IsPromptTooLong(err), "preserve the underlying provider error")
			require.NoError(t, ctx.Err(), "overflow must stop immediately, not by timeout")
			require.Equal(t, 1, calls, "must not retry at either the request or outer-loop level")
			require.Zero(t, compressionCalls)
			require.Equal(t, before, yakit.GetAIModelContext(db, "test", model))
			require.Nil(t, react.config.ModelContextStatus(0).Change)
		})
	}
}
