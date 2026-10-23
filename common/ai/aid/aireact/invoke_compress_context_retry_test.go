package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func compressionActionResponse(t *testing.T, cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest, name string, params map[string]any) *aicommon.AIResponse {
	t.Helper()
	response := cfg.NewAIResponse()
	wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
	if wire.ToolCallCallback != nil {
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: uuid.NewString(), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(encoded)}}})
		wire.FinishReasonCallback("tool_calls", nil)
	} else {
		params["@action"] = name
		encoded, err := json.Marshal(params)
		require.NoError(t, err)
		response.EmitOutputStream(strings.NewReader(string(encoded)))
	}
	response.Close()
	return response
}

func TestReAct_CompressContextSelectionRetry(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, invalid := range []string{"unknown_id", "unknown_skill", "retention_budget", "retry_exhausted", "normal_turn"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, invalid), func(t *testing.T) {
				var attempts atomic.Int32
				var history []string
				summary := uuid.NewString()
				forced := invalid != "normal_turn"
				limit := 300 * 1024
				if forced {
					limit = 60 * 1024
				}
				react, err := NewTestReAct(
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithTimelineContentLimit(limit),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(2),
					aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
					aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
							return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
						}
						if req.GetCallerLabel() == aicommon.CallerLabelContextCompress || !forced && attempts.Load() == 0 {
							attempt := attempts.Add(1)
							params := map[string]any{
								"summary": summary, "retain_timeline": compressionHistoryIDFromPrompt(t, req.GetPrompt(), history[len(history)-1]),
								"retire_skill_names": []string{}, "retire_evidence_ids": []string{},
							}
							if attempt == 1 || invalid == "retry_exhausted" {
								switch invalid {
								case "unknown_skill":
									params["retire_skill_names"] = []string{"missing-skill-" + uuid.NewString()}
								case "retention_budget":
									params["retain_timeline"] = compressionHistoryIDFromPrompt(t, req.GetPrompt(), history[0]) + "-" + params["retain_timeline"].(string)
								default:
									params["retain_timeline"] = "999999999"
								}
							}
							if attempt > 1 {
								require.Contains(t, req.GetPrompt(), "# 上次压缩选择未通过校验")
								require.Equal(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel())
							}
							return compressionActionResponse(t, cfg, req, aicommon.ContextCompressionAction, params), nil
						}
						if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
							return nil, fmt.Errorf("unexpected auxiliary request %q", req.GetCallerLabel())
						}
						require.EqualValues(t, 2, attempts.Load(), "business work must wait for a valid compression selection")
						return compressionActionResponse(t, cfg, req, "finish", map[string]any{}), nil
					}))
				require.NoError(t, err)
				for i := 0; i < 4; i++ {
					marker := uuid.NewString()
					history = append(history, marker)
					padding := ""
					if forced {
						words := 900
						if i == 3 {
							words = 100
						}
						padding = strings.Repeat(uuid.NewString()+" ", words)
					}
					react.config.Timeline.PushText(react.config.AcquireId(), "%s %s", marker, padding)
				}
				loop, err := reactloops.NewReActLoop("compression-selection-retry", react,
					reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(2))
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				err = loop.Execute("selection-retry-task", ctx, "Compact the context and continue.")
				require.EqualValues(t, 2, attempts.Load())
				view := aicommon.RenderTimelineFrozenOpen(react.config.Timeline).PromptText()
				if invalid == "retry_exhausted" {
					require.ErrorContains(t, err, "selection rejected after 2 attempts")
					require.Contains(t, view, history[0], "failed proposals must leave the original history intact")
				} else {
					require.NoError(t, err)
					require.Contains(t, view, summary)
					require.Contains(t, view, history[len(history)-1])
					require.NotContains(t, view, history[0])
				}
			})
		}
	}
}
