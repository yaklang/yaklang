package liteforge_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	_ "github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
)

func TestLiteForgePreservesParentCompressionBinding(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "read-history", true: "without-history"}[disabled], func(t *testing.T) {
			calls := 0
			var parent *aicommon.Config
			callback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				calls++
				text := `{"@action":"probe","value":"ok"}`
				if req.GetCallerLabel() == aicommon.CallerLabelTimelineCompress {
					text = `{"@action":"timeline-summary","summary":"verified finding","ratain_timeline_item_range":"","memory_entities":[]}`
				} else {
					if disabled {
						require.NotContains(t, req.GetPrompt(), "PARENT_FINDING")
					} else {
						require.Contains(t, req.GetPrompt(), "PARENT_FINDING")
					}
				}
				resp := aicommon.NewUnboundAIResponse()
				resp.EmitOutputStream(strings.NewReader(text))
				resp.Close()
				return resp, nil
			}
			parent = aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithTimelineContentLimit(1), aicommon.WithSpeedPriorityAICallback(callback))
			parent.Timeline.PushText(1, "PARENT_FINDING")
			before, err := aicommon.MarshalTimeline(parent.Timeline)
			require.NoError(t, err)
			request := &aicommon.LiteForgeInvokeRequest{Context: context.Background(), ActionName: "probe", OutputActionName: "probe",
				OutputSchema: `{"type":"object","properties":{"@action":{"const":"probe"},"value":{"type":"string"}},"required":["@action","value"]}`}
			if disabled {
				request.Options = append(request.Options, aicommon.WithLiteForgeDisableTimeline())
			}
			opts := []any{request}
			_, err = parent.InvokeLiteForge("single step", opts...)
			require.NoError(t, err)
			after, err := aicommon.MarshalTimeline(parent.Timeline)
			require.NoError(t, err)
			require.Equal(t, before, after)
			// A new helper Config defaults to 50K. If it rebinds the parent,
			// this check no longer triggers and the live compression is broken.
			result, err := parent.Timeline.CompressBeforePrompt(aicommon.TimelineCompressionOptions{})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "verified finding", result.Summary)
			require.Equal(t, 2, calls)
		})
	}
}

// Exercise the registered production LiteForge engine, not the scoped Timeline
// unit-test adapter: a complete prompt schema must not reject legacy summaries.
func TestTimelineCompressionSummaryCompatibilityThroughLiteForge(t *testing.T) {
	for _, test := range []struct {
		name, response string
		valid          bool
	}{
		{"summary-only", `{"summary":"顶层验证已通过；继续等待嵌套结果。"}`, true},
		{"fenced", "说明：\n```json\n" + `{"summary":"顶层验证已通过；继续等待嵌套结果。"}` + "\n```", true},
		{"optional-invalid", `{"summary":"顶层验证已通过；继续等待嵌套结果。","memory_entities":"unavailable","ratain_timeline_item_range":[1]}`, true},
		{"empty", `{"summary":" "}`, false},
		{"wrong-summary-type", `{"summary":false}`, false},
		{"nested-summary", `{"metadata":{"summary":"nested"}}`, false},
		{"incomplete", `{"summary":"unfinished"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithSpeedPriorityAICallback(func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls++
					require.Contains(t, req.GetPrompt(), `"memory_entities"`)
					require.Contains(t, req.GetPrompt(), `"ratain_timeline_item_range"`)
					resp := aicommon.NewUnboundAIResponse()
					resp.EmitOutputStream(strings.NewReader(test.response))
					resp.Close()
					return resp, nil
				}))
			cfg.Timeline.PushText(1, "history before compression")
			before, err := aicommon.MarshalTimeline(cfg.Timeline)
			require.NoError(t, err)
			result, err := cfg.Timeline.CompressOnce(aicommon.TimelineCompressionOptions{
				MaxInputTokens: aicommon.TimelineCompressionMaxInputTokens, MaxSummaryTokens: aicommon.TimelineCompressionMaxSummaryTokens,
			})
			if test.valid {
				require.NoError(t, err)
				require.NotEmpty(t, result.Summary)
				require.Empty(t, result.RetainedIDs)
			} else {
				require.Error(t, err)
				after, err := aicommon.MarshalTimeline(cfg.Timeline)
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
			require.Equal(t, 1, calls)
		})
	}
}
