package liteforge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiengine"
)

func liteForgeResponse(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, native bool, value any) *aicommon.AIResponse {
	if !native {
		copy := make(map[string]any)
		for key, item := range value.(map[string]any) {
			copy[key] = item
		}
		if _, exists := copy["@action"]; !exists {
			copy["@action"] = "result"
		}
		value = copy
	}
	data, _ := json.Marshal(value)
	resp := c.NewAIResponse()
	if native {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		projected := aiprojection.ProjectAndObserve("liteforge-yak-test", req.GetPrompt())
		wire.ToolCallCallback([]*aispec.ToolCall{{ID: "result", Function: aispec.FuncReturn{Name: projected.Tools[0].Function.Name, Arguments: string(data)}}})
		wire.ToolCallArgumentsStreamHandler(strings.NewReader(string(data)))
		wire.FinishReasonCallback("tool_calls", nil)
	} else {
		resp.EmitOutputStream(strings.NewReader(string(data)))
	}
	resp.Close()
	return resp
}

func TestLiteForgeAIMDefaultsToTextAndPublicBothProtocols(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, task := range []string{"extract", "classify", "summarize"} {
			t.Run(fmt.Sprintf("%s/native_%v", task, native), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				schema := `{"type":"object","properties":{"summary":{"type":"string"},"payload":{}},"required":["summary","payload"],"additionalProperties":true}`
				value := map[string]any{"summary": task + " 已完成", "payload": map[string]any{"items": []any{nil, true, 2.5}}, "business_extension": map[string]any{"source": "Yak+aim"}}
				calls := 0
				model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls++
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					require.Empty(t, wire.Tools, "tools must be supplied by projection")
					projected := aiprojection.ProjectAndObserve("liteforge-yak-test", req.GetPrompt())
					// ReAct's helper stays text even when the parent loop is native.
					wantNative := calls == 2 && native
					require.Equal(t, wantNative, len(projected.Tools) == 1)
					require.Equal(t, "system", projected.Messages[0].Role)
					return liteForgeResponse(c, req, wantNative, value), nil
				}
				config := []aicommon.ConfigOption{aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true),
					aicommon.WithDisablePerception(true), aicommon.WithNoOpMemoryTriage(), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithEnableFunctionCallMode(native), aicommon.WithWorkdir(t.TempDir()), aicommon.WithLiteForgeExecutor(liteforge.ExecuteTyped)}
				verify := func(action *aicommon.Action) {
					require.Equal(t, "result", action.ActionType())
					expected := make(map[string]any)
					for k, v := range value {
						expected[k] = v
					}
					expected["@action"] = "result"
					data, _ := json.Marshal(expected)
					actual, _ := json.Marshal(action.GetParams())
					require.JSONEq(t, string(data), string(actual))
				}
				engine, err := aiengine.NewAIEngine(aiengine.WithAICallback(model), aiengine.WithExtOptions(config...))
				require.NoError(t, err)
				defer engine.Close()
				action, err := engine.GetOperator().(*aireact.ReAct).InvokeLiteForge(ctx, "result", task+" 输入材料", []aitool.ToolOption{aitool.WithStringParam("summary", aitool.WithParam_Required()), aitool.WithRawParam("payload", map[string]any{}, aitool.WithParam_Required())})
				require.NoError(t, err)
				verify(action)
				require.Equal(t, native, engine.GetOperator().(*aireact.ReAct).GetConfig().GetConfigBool("EnableFunctionCallMode"))
				forge, err := liteforgeapp.NewLiteForge("result", liteforgeapp.WithLiteForge_Prompt(task+" 输入材料"), liteforgeapp.WithLiteForge_OutputSchemaRaw("result", schema))
				require.NoError(t, err)
				result, err := forge.Execute(ctx, nil, append(config, aicommon.WithAICallback(model))...)
				require.NoError(t, err)
				verify(result.Action)
				require.NoError(t, err)
				require.Equal(t, 2, calls, "one model request per public/aim entry")
			})
		}
	}
}

func TestLiteForgeLegacyEnvelope(t *testing.T) {
	for _, native := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		forge, err := liteforgeapp.NewLiteForge("legacy-envelope", liteforgeapp.WithLiteForge_OutputSchema(aitool.WithStringParam("summary", aitool.WithParam_Required())))
		require.NoError(t, err)
		result, err := forge.Execute(ctx, nil, aicommon.WithAITransactionAutoRetry(1), aicommon.WithEnableFunctionCallMode(native), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			value := map[string]any{"tool": "output", "params": map[string]any{"summary": "兼容嵌套参数"}}
			if !native {
				value["@action"] = "call-tool"
			}
			return liteForgeResponse(c, req, native, value), nil
		}))
		require.NoError(t, err)
		require.Equal(t, "call-tool", result.Action.ActionType())
		require.Equal(t, "兼容嵌套参数", result.Action.GetString("summary"))
		require.Equal(t, "兼容嵌套参数", result.Action.GetInvokeParams("params").GetString("summary"))
		cancel()
	}
}

func TestLiteForgePublicDefaultAndProtocolOverride(t *testing.T) {
	for _, tc := range []struct {
		name    string
		native  bool
		options []aicommon.ConfigOption
	}{
		{"default", false, nil},
		{"native", true, []aicommon.ConfigOption{aicommon.WithEnableFunctionCallMode(true)}},
		{"text", false, []aicommon.ConfigOption{aicommon.WithEnableFunctionCallMode(false)}},
		{"last_option_wins", true, []aicommon.ConfigOption{aicommon.WithEnableFunctionCallMode(false), aicommon.WithEnableFunctionCallMode(true)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			forge, err := liteforgeapp.NewLiteForge("default-protocol", liteforgeapp.WithLiteForge_OutputSchemaRaw("result",
				`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"]}`))
			require.NoError(t, err)
			options := []aicommon.ConfigOption{aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				projected := aiprojection.ProjectAndObserve("liteforge-default-test", req.GetPrompt())
				require.Equal(t, tc.native, len(projected.Tools) == 1)
				return liteForgeResponse(c, req, tc.native, map[string]any{"summary": "默认及覆盖选项生效"}), nil
			})}
			result, err := forge.Execute(ctx, nil, append(options, tc.options...)...)
			require.NoError(t, err)
			require.Equal(t, "默认及覆盖选项生效", result.Action.GetString("summary"))
		})
	}
}

func TestLiteForgeConfigAndAuxiliaryDoNotInheritProtocol(t *testing.T) {
	for _, parentNative := range []bool{false, true} {
		t.Run(fmt.Sprint(parentNative), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			wantNative, calls := false, 0
			cfg := aicommon.NewConfig(ctx, aicommon.WithEnableFunctionCallMode(parentNative),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true),
				aicommon.WithNoOpMemoryTriage(), aicommon.WithSingleAIModelMode(false), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithLiteForgeExecutor(liteforge.ExecuteTyped),
				aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					calls++
					projected := aiprojection.ProjectAndObserve("config-helper-protocol", req.GetPrompt())
					require.Equal(t, wantNative, len(projected.Tools) == 1)
					return liteForgeResponse(c, req, wantNative, map[string]any{"@action": "result", "summary": "已验证"}), nil
				}))
			request := &aicommon.LiteForgeInvokeRequest{Context: ctx, ActionName: "result",
				Outputs: []aitool.ToolOption{aitool.WithStringParam("summary", aitool.WithParam_Required())}}
			result, err := cfg.InvokeLiteForge("检查默认协议", request)
			require.NoError(t, err)
			require.Equal(t, "已验证", result.Action.GetString("summary"))
			for _, name := range []string{aicommon.CallerLabelMemoryTriage, aicommon.CallerLabelMiniTimelineSummary} {
				completed := false
				cfg.ScheduleAuxiliaryTask(ctx, name, func() string { return "检查辅助协议" }, func(action *aicommon.Action) {
					completed = true
					require.Equal(t, "已验证", action.GetString("summary"))
				}, aicommon.WithAuxiliaryOutputSchema("result", `{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"]}`),
					aicommon.WithAuxiliaryOnError(func(err error) { require.NoError(t, err) }))
				require.True(t, completed)
			}
			wantNative = true
			result, err = cfg.InvokeLiteForge("显式启用原生协议", request, aicommon.WithEnableFunctionCallMode(true))
			require.NoError(t, err)
			require.Equal(t, "已验证", result.Action.GetString("summary"))
			require.Equal(t, 4, calls)
			require.Equal(t, parentNative, cfg.EnableFunctionCallMode, "child calls must not modify the parent")
		})
	}
}

func TestLiteForgeCustomHandlerStillOwnsPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	forge, err := liteforgeapp.NewLiteForge("custom", liteforgeapp.WithLiteForge_Prompt("raw custom prompt"),
		liteforgeapp.WithLiteForge_ResponseHandler(func(resp *aicommon.AIResponse) (*aicommon.Action, error) {
			data, err := io.ReadAll(resp.GetUnboundStreamReader(false))
			if err != nil {
				return nil, err
			}
			return aicommon.ExtractAction(string(data), "custom")
		}))
	require.NoError(t, err)
	_, err = forge.Execute(ctx, nil, aicommon.WithAITransactionAutoRetry(1), aicommon.WithEnableFunctionCallMode(true),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			require.Equal(t, "raw custom prompt", req.GetPrompt())
			require.Empty(t, aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...).Tools)
			return liteForgeResponse(c, req, false, map[string]any{"@action": "custom"}), nil
		}))
	require.NoError(t, err)
}
