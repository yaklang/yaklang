package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

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
