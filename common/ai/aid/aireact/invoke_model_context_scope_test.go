package aireact

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_ModelContextPersistsOriginalRecoveryOnlyAfterSuccess(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, disabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%v/disabled=%v", native, disabled), func(t *testing.T) {
				model := &ypb.AIModelConfig{ModelName: uuid.NewString(), Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: "https://example.test/v1", APIKey: "test-only"}}
				registerModelContextTestConfig(t, model)
				key := aispec.ModelContextIdentity(model)
				db := consts.GetGormProfileDatabase()
				if disabled {
					require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", model.ModelName, 500000, &disabled, key))
				}
				calls := 0
				react, err := NewTestReAct(
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1), aicommon.WithPrimaryModelContext(model),
					aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
							return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
						}
						calls++
						require.EqualValues(t, 500000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens, "a rejected request must not persist")
						if calls == 1 {
							return nil, &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "openai", Model: model.ModelName, ContextKey: key, MaxContextTokens: 128000}}
						}
						require.NotEqual(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel(), "no new first-failure prefix compression path")
						response := compressionActionResponse(t, cfg, req, "finish", map[string]any{})
						response.SetModelInfo("openai", model.ModelName)
						response.SetModelContextKey(key)
						return response, nil
					}))
				require.NoError(t, err)
				loop, err := reactloops.NewReActLoop("model-context-recovery", react, reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(3))
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				err = loop.Execute(uuid.NewString(), ctx, "test")
				if disabled {
					require.ErrorContains(t, err, "自动调整已关闭")
					require.Equal(t, 1, calls)
					require.EqualValues(t, 500000, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)
				} else {
					require.NoError(t, err)
					require.Equal(t, 2, calls)
					require.EqualValues(t, 102400, yakit.GetAIModelContext(db, "openai", model.ModelName, key).ThresholdTokens)
				}
			})
		}
	}
}

func registerModelContextTestConfig(t *testing.T, model *ypb.AIModelConfig) {
	t.Helper()
	db := consts.GetGormProfileDatabase()
	cfg, err := yakit.GetAIGlobalConfig(db)
	require.NoError(t, err)
	if cfg == nil {
		cfg = &ypb.AIGlobalConfig{}
	}
	cfg.IntelligentModels = append(cfg.IntelligentModels, model)
	_, err = yakit.SetAIGlobalConfig(db, cfg)
	require.NoError(t, err)
}
