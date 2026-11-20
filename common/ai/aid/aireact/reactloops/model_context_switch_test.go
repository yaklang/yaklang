package reactloops

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func selectContextTestModel(model *ypb.AIModelConfig) {
	consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, IntelligentConfigs: []*ypb.AIModelConfig{model}})
}

func setContextTestThreshold(t *testing.T, cfg *aicommon.Config, key string, tokens int64) {
	t.Helper()
	opts := cfg.ProcessHotPatchMessage(&ypb.AIInputEvent{
		IsConfigHotpatch: true, HotpatchType: aicommon.HotPatchType_RuntimeContextSizeLimit,
		Params: &ypb.AIStartParams{ModelContextKey: key, RuntimeContextSizeLimit: tokens},
	})
	require.Len(t, opts, 1)
	require.NoError(t, opts[0](cfg))
}

// Exercise the real tier callback and HTTP gateway for both ReAct protocols:
// build/check A's threshold, pause immediately before gateway model selection,
// select/edit B, resume sending, then compress/retry A and start the next round B.
// Channels force the TOCTOU interleaving without sleeps or scheduler timing.
// Identical display names deliberately rule out name-only identity checks.
func TestModelContextTOCTOUPinsPromptGatewayAndCompression(t *testing.T) {
	for _, tc := range []struct {
		native    bool
		childMode string
	}{{false, ""}, {true, ""}, {false, SubAgentContextFork}, {true, SubAgentContextFork}, {false, SubAgentContextTaskOnly}, {true, SubAgentContextTaskOnly}} {
		t.Run(fmt.Sprintf("native=%v/child=%s", tc.native, tc.childMode), func(t *testing.T) {
			native := tc.native
			yakit.GuardConfigurationForTest(t)
			var aCalls, bCalls atomic.Int32
			server := func(count *atomic.Int32, rejectFirst bool) *httptest.Server {
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if count.Add(1) == 1 && rejectFirst {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"maximum context length is 200000 tokens"}}`)
						return
					}
					params := map[string]any{"summary": "summary", "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}}
					var delta map[string]any
					finish := "stop"
					if native {
						arguments, _ := json.Marshal(params)
						delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "compress-1", "type": "function", "function": map[string]any{"name": aicommon.ContextCompressionAction, "arguments": string(arguments)}}}}
						finish = "tool_calls"
					} else {
						params["@action"] = aicommon.ContextCompressionAction
						content, _ := json.Marshal(params)
						delta = map[string]any{"content": string(content)}
					}
					chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
				}))
				t.Cleanup(s.Close)
				return s
			}
			model := func(url string) *ypb.AIModelConfig {
				return &ypb.AIModelConfig{ModelName: "same-model", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: url + "/v1", APIKey: "test-only"}}
			}
			a, b := model(server(&aCalls, true).URL), model(server(&bCalls, false).URL)
			registerModelContextTestConfig(t, a)
			registerModelContextTestConfig(t, b)
			aKey, bKey := aispec.ModelContextIdentity(a), aispec.ModelContextIdentity(b)
			require.NotEqual(t, aKey, bKey)
			selectContextTestModel(a)
			_, cfg := newPromptOverflowRetryTestLoop(t, aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1))
			require.NoError(t, aicommon.WithTieredAICallback(false)(cfg))
			route := cfg.GetRawAICallbacks().QualityPriorityRaw
			require.NotNil(t, route)
			requestReady, allowRequest := make(chan struct{}), make(chan struct{})
			var pauseOnce, releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(allowRequest) }) }
			t.Cleanup(release)
			require.NoError(t, aicommon.WithQualityPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				pauseOnce.Do(func() {
					close(requestReady)
					select {
					case <-allowRequest:
					case <-c.GetContext().Done():
					}
				})
				return route(c, req)
			})(cfg))
			if tc.childMode != "" {
				cfg = newModelContextTestSubAgent(t, cfg, tc.childMode)
			}
			loop := makeSchemaStabilityTestLoop(cfg)
			loop.SetCurrentTask(cfg.DefaultTask)
			t.Cleanup(loop.Release)
			loop.functionCallMode = native
			loop.actions.Set(aicommon.ContextCompressionAction, loopAction_CompressContext)
			loop.invoker = &compressionPromptInvoker{MockInvoker: mock.NewMockInvoker(cfg.GetContext()), timeline: cfg.Timeline}
			cfg.Timeline.PushText(cfg.AcquireId(), "history to compress")
			setContextTestThreshold(t, cfg, aKey, 400000)
			operator := &LoopActionHandlerOperator{}
			input := loop.captureLoopPromptBuildInput("binding", "task", "context", nil, "", operator)

			build := func(input *loopPromptBuildInput, want int64) *LoopPromptAssemblyResult {
				result, err := loop.generateLoopPromptFromInput(input)
				require.NoError(t, err)
				require.Equal(t, want, loop.wholePromptTokenLimit())
				require.Equal(t, want, cfg.Timeline.GetTimelineContentLimit())
				loop.promptMode = result.Mode
				return result
			}
			call := func(result *LoopPromptAssemblyResult) (*LoopResultDescriptor, error) {
				var streams sync.WaitGroup
				_, _, descriptor, err := loop.callAILoopTransaction(&streams, result.Prompt, "binding", operator, loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
				streams.Wait()
				return descriptor, err
			}
			first := build(input, 400000)
			result := make(chan error, 1)
			go func() {
				_, err := call(first)
				result <- err
			}()
			select {
			case <-requestReady:
			case <-cfg.GetContext().Done():
				t.Fatal("request did not reach the gateway selection barrier")
			}
			// This is the exact check/use gap: the prompt was checked against A,
			// but the old tier callback would resolve B when released below.
			selectContextTestModel(b)
			setContextTestThreshold(t, cfg, bKey, 300000)
			require.EqualValues(t, 400000, loop.wholePromptTokenLimit())
			require.Zero(t, aCalls.Load())
			require.Zero(t, bCalls.Load())
			release()
			var err error
			select {
			case err = <-result:
			case <-cfg.GetContext().Done():
				t.Fatal("request did not finish after releasing the gateway selection barrier")
			}
			t.Logf("TOCTOU: checked A at 400000, selected B at 300000 before sending; gateway calls A=%d B=%d", aCalls.Load(), bCalls.Load())
			require.Zero(t, bCalls.Load(), "TOCTOU: B must not receive a prompt checked against A's threshold")
			require.EqualValues(t, 1, aCalls.Load())
			require.True(t, aicommon.IsPromptTooLong(err), "%v", err)
			evidence, ok := aicommon.PromptOverflowEvidenceFrom(err)
			require.True(t, ok)
			require.Equal(t, aKey, evidence.ContextKey)
			require.True(t, loop.reduceTimelineLimitForPromptOverflow(err, first.Prompt))
			db := consts.GetGormProfileDatabase()
			require.EqualValues(t, 400000, yakit.GetAIModelContext(db, "openai", a.ModelName, aKey).ThresholdTokens, "overflow alone must not persist")
			input.overflowCompression = true
			compressed := build(input, 160000)
			require.Equal(t, aicommon.LoopPromptModeCompressContext, compressed.Mode)
			descriptor, err := call(compressed)
			require.NoError(t, err)
			require.Equal(t, aKey, descriptor.Snapshot().ContextKey)
			loop.confirmModelContextSuccess(descriptor, int64(aicommon.MeasureTokens(compressed.Prompt)))
			require.EqualValues(t, 2, aCalls.Load())
			require.Zero(t, bCalls.Load())
			require.EqualValues(t, 160000, yakit.GetAIModelContext(db, "openai", a.ModelName, aKey).ThresholdTokens)
			require.EqualValues(t, 300000, yakit.GetAIModelContext(db, "openai", b.ModelName, bKey).ThresholdTokens)

			next := loop.captureLoopPromptBuildInput("binding", "next task", "context", nil, "", operator)
			descriptor, err = call(build(next, 300000))
			require.NoError(t, err)
			require.Equal(t, bKey, descriptor.Snapshot().ContextKey)
			require.EqualValues(t, 1, bCalls.Load())
			t.Log("A: prompt/request 400000 -> overflow -> compression/request 160000 -> persisted 160000; B: manual 300000 unchanged, next request uses B")
		})
	}
}

func TestModelContextLateOverflowUsesRequestAutoAdjustSetting(t *testing.T) {
	for _, disabledA := range []bool{false, true} {
		t.Run(fmt.Sprintf("disabledA=%v", disabledA), func(t *testing.T) {
			yakit.GuardConfigurationForTest(t)
			model := func(endpoint string) *ypb.AIModelConfig {
				return &ypb.AIModelConfig{ModelName: "same-model", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: endpoint}}
			}
			a, b := model("https://first.example.test/v1"), model("https://second.example.test/v1")
			registerModelContextTestConfig(t, a)
			registerModelContextTestConfig(t, b)
			aKey, bKey := aispec.ModelContextIdentity(a), aispec.ModelContextIdentity(b)
			db := consts.GetGormProfileDatabase()
			disabledB := !disabledA
			require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", a.ModelName, 400000, &disabledA, aKey))
			require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", b.ModelName, 300000, &disabledB, bKey))
			selectContextTestModel(b)
			loop, cfg := newPromptOverflowRetryTestLoop(t)
			require.NoError(t, aicommon.WithTieredAICallback(false)(cfg))
			overflow := &aispec.PromptOverflowError{PromptOverflowEvidence: aispec.PromptOverflowEvidence{Provider: "openai", Model: a.ModelName, ContextKey: aKey, MaxContextTokens: 200000}}
			require.Equal(t, disabledA, loop.promptOverflowAutoAdjustDisabled(overflow))
			require.Equal(t, !disabledA, loop.reduceTimelineLimitForPromptOverflow(overflow, "old A request"))
			loop.syncPromptOverflowTimelineLimit()
			require.EqualValues(t, 300000, loop.wholePromptTokenLimit())
			// B's success must not confirm A's pending adjustment.
			loop.confirmModelContextSuccess(&LoopResultDescriptor{snapshot: LoopResultSnapshot{Provider: "openai", Model: b.ModelName, ContextKey: bKey}}, 1000)
			require.EqualValues(t, 400000, yakit.GetAIModelContext(db, "openai", a.ModelName, aKey).ThresholdTokens)
			loop.confirmModelContextSuccess(&LoopResultDescriptor{snapshot: LoopResultSnapshot{Provider: "openai", Model: a.ModelName, ContextKey: aKey}}, 1000)
			wantA := int64(160000)
			if disabledA {
				wantA = 400000
			}
			require.EqualValues(t, wantA, yakit.GetAIModelContext(db, "openai", a.ModelName, aKey).ThresholdTokens)
			require.EqualValues(t, 300000, yakit.GetAIModelContext(db, "openai", b.ModelName, bKey).ThresholdTokens)
		})
	}
}
