package reactloops

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestContextCompressionPreservesConfiguredThinking(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, level := range []string{"low", "high", "none"} {
			t.Run(fmt.Sprintf("native=%v/level=%s", native, level), func(t *testing.T) {
				ctx := context.Background()
				var observed []string
				cfg := aicommon.NewConfig(ctx,
					aicommon.WithDisableAutoSkills(true), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithQualityPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						// The provider's configured level is applied before request
						// overrides, just as in the gateway. Switching prompt mode
						// must not silently change that effective configuration.
						opts := []aispec.AIConfigOption{aispec.WithThinkingLevel(level)}
						opts = append(opts, req.GetExtraSpecOpts()...)
						wire := aispec.NewDefaultAIConfig(opts...)
						observed = append(observed, wire.ThinkingLevel)
						name := "accept"
						params := map[string]any{"text": "business step"}
						if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
							name = aicommon.ContextCompressionAction
							params = map[string]any{
								"summary": "critical evidence retained", "retain_timeline": "",
								"retire_skill_names": []string{}, "retire_evidence_ids": []string{},
							}
						}
						return contextCompressionTestResponse(t, c, req, name, params, false), nil
					}))
				loop := makeSchemaStabilityTestLoop(cfg)
				loop.functionCallMode, loop.useSpeedPriorityAI = native, false
				invoker := mock.NewMockInvoker(ctx)
				invoker.SetConfig(cfg)
				loop.invoker = invoker
				loop.actions.Set("accept", &LoopAction{ActionType: "accept", Options: []aitool.ToolOption{aitool.WithStringParam("text")}})
				loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
				_, _, err := loop.prepareLoopActionSchemas(&LoopActionHandlerOperator{})
				require.NoError(t, err)
				for _, mode := range []aicommon.LoopPromptMode{aicommon.LoopPromptModeNormal, aicommon.LoopPromptModeCompressContext} {
					loop.promptMode = mode
					var streams sync.WaitGroup
					calls, _, _, err := loop.callAILoopTransaction(&streams, "stable context", "thinking", &LoopActionHandlerOperator{}, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
					streams.Wait()
					require.NoError(t, err)
					require.Len(t, calls, 1)
				}
				require.Equal(t, []string{level, level}, observed,
					"normal and compression requests must preserve the same configured thinking level, including an explicit none")
			})
		}
	}
}
