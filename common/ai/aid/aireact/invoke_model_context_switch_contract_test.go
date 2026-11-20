package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// A complete Execute run, with real prompt assembly, HTTP gateways, compression
// application and model_context events. No test calls the recovery helpers or
// sets overflowCompression: the production loop must choose the right branch.
func TestReAct_ModelContextSwitchFittingBoundaryContract(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, scenario := range []string{"compression_success", "compression_rejected", "request_model_auto_adjust_disabled"} {
			t.Run(fmt.Sprintf("native=%v/%s", native, scenario), func(t *testing.T) {
				yakit.GuardConfigurationForTest(t)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				t.Cleanup(cancel)
				oldHistory, lateHistory, summary := uuid.NewString(), uuid.NewString(), uuid.NewString()
				overflowArrived, releaseOverflow := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(releaseOverflow) }) }
				t.Cleanup(release)
				type request struct{ role, body string }
				var mu sync.Mutex
				var requests []request
				var events []aicommon.ModelContextStatus
				var eventErrors []error
				aCalls := 0
				server := func(role string) *httptest.Server {
					s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						body, err := io.ReadAll(req.Body)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						mu.Lock()
						requests = append(requests, request{role, string(body)})
						if role == "A" {
							aCalls++
						}
						call := aCalls
						mu.Unlock()
						if role == "A" && call == 2 {
							close(overflowArrived)
							select {
							case <-releaseOverflow:
							case <-ctx.Done():
								return
							}
						}
						if role == "A" && (call == 2 || call == 3 && scenario == "compression_rejected") {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprint(w, `{"error":{"code":"context_length_exceeded","message":"maximum context length is 200000 tokens"}}`)
							return
						}
						name, params := "finish", map[string]any{}
						if role == "A" {
							if call == 1 {
								name = "probe"
							} else {
								name = aicommon.ContextCompressionAction
								params = map[string]any{"summary": summary, "retain_timeline": "", "retire_skill_names": []string{}, "retire_evidence_ids": []string{}}
							}
						}
						writeModelContextAction(w, native, name, params)
					}))
					t.Cleanup(s.Close)
					return s
				}
				model := func(url string) *ypb.AIModelConfig {
					return &ypb.AIModelConfig{ModelName: "same-model", Provider: &ypb.ThirdPartyApplicationConfig{Type: "openai", BaseURL: url + "/v1", APIKey: "test-only"}}
				}
				a, b := model(server("A").URL), model(server("B").URL)
				registerModelContextTestConfig(t, a)
				registerModelContextTestConfig(t, b)
				aKey, bKey := aispec.ModelContextIdentity(a), aispec.ModelContextIdentity(b)
				require.NotEqual(t, aKey, bKey)
				selectModel := func(model *ypb.AIModelConfig) {
					consts.SetTieredAIConfig(&consts.TieredAIConfig{Enabled: true, IntelligentConfigs: []*ypb.AIModelConfig{model}})
				}
				selectModel(a)
				db := consts.GetGormProfileDatabase()
				disabledA := scenario == "request_model_auto_adjust_disabled"
				disabledB := !disabledA
				require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", a.ModelName, 400000, &disabledA, aKey))
				require.NoError(t, yakit.SetAIModelContextSettings(db, "openai", b.ModelName, 300000, &disabledB, bKey))
				auxiliary := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if !aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected auxiliary request %q", req.GetCallerLabel())
					}
					return compressionActionResponse(t, cfg, req, "verify-satisfaction", map[string]any{"user_satisfied": true, "reasoning": "done"}), nil
				}
				react, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1), aicommon.WithAICallback(auxiliary),
					aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
						if event.Type != schema.EVENT_TYPE_MODEL_CONTEXT {
							return
						}
						var status aicommon.ModelContextStatus
						err := json.Unmarshal(event.Content, &status)
						mu.Lock()
						defer mu.Unlock()
						if err != nil {
							eventErrors = append(eventErrors, err)
						} else {
							events = append(events, status)
						}
					}))
				require.NoError(t, err)
				require.NoError(t, aicommon.WithTieredAICallback(false)(react.config))
				require.NoError(t, aicommon.WithSpeedPriorityAICallback(auxiliary)(react.config))
				react.config.Timeline.PushText(react.config.AcquireId(), "%s", oldHistory)
				loop, err := reactloops.NewReActLoop("switch-fitting-context", react,
					reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4),
					reactloops.WithRegisterLoopAction("probe", "record history after an accepted request", nil, nil,
						func(_ *reactloops.ReActLoop, _ *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
							react.config.Timeline.PushText(react.config.AcquireId(), "%s", lateHistory)
							op.Continue()
						}))
				require.NoError(t, err)
				result, finished := make(chan error, 1), make(chan struct{})
				go func() {
					defer close(finished)
					result <- loop.Execute(uuid.NewString(), ctx, "Continue the task.")
				}()
				t.Cleanup(func() { cancel(); <-finished; loop.Release() })
				select {
				case <-overflowArrived:
				case <-ctx.Done():
					t.Fatal("A did not reach the second request after establishing a fitting boundary")
				}
				selectModel(b)
				opts := react.config.ProcessHotPatchMessage(&ypb.AIInputEvent{IsConfigHotpatch: true,
					HotpatchType: aicommon.HotPatchType_RuntimeContextSizeLimit,
					Params:       &ypb.AIStartParams{ModelContextKey: bKey, RuntimeContextSizeLimit: 300000}})
				require.Len(t, opts, 1)
				require.NoError(t, opts[0](react.config))
				release()
				err = <-result
				if disabledA {
					require.ErrorContains(t, err, "自动调整已关闭")
				} else {
					require.NoError(t, err)
				}
				// The fitting-boundary branch compresses content without learning a
				// smaller threshold, even though the rejection reports a maximum.
				savedA, savedB := yakit.GetAIModelContext(db, "openai", a.ModelName, aKey), yakit.GetAIModelContext(db, "openai", b.ModelName, bKey)
				require.EqualValues(t, 400000, savedA.ThresholdTokens)
				require.EqualValues(t, 300000, savedB.ThresholdTokens)
				require.Equal(t, "manual", savedA.Source)
				require.Equal(t, disabledA, savedA.DisableAutoAdjust)
				require.Equal(t, disabledB, savedB.DisableAutoAdjust)
				mu.Lock()
				defer mu.Unlock()
				require.Empty(t, eventErrors)
				require.NotEmpty(t, events)
				var runtimeA, runtimeB bool
				for _, event := range events {
					require.False(t, event.Pending, "boundary compression must not stage an adaptive threshold")
					if event.ContextKey == aKey {
						require.EqualValues(t, 400000, event.ThresholdTokens)
						runtimeA = runtimeA || event.PromptTokens > 0
					} else {
						require.Equal(t, bKey, event.ContextKey)
						require.EqualValues(t, 300000, event.ThresholdTokens)
						runtimeB = runtimeB || event.PromptTokens > 0
					}
				}
				require.True(t, runtimeA, "A requests must publish A's context status")
				require.Equal(t, !disabledA, runtimeB, "B runtime status appears only after actually continuing with B")
				require.GreaterOrEqual(t, len(requests), 2)
				require.Contains(t, requests[0].body, oldHistory)
				require.NotContains(t, requests[0].body, lateHistory)
				require.Contains(t, requests[1].body, lateHistory, "the rejected request includes history after the fitting boundary")
				if disabledA {
					require.Len(t, requests, 2, "A disabled: stop immediately, despite B having auto-adjust enabled")
					return
				}
				require.Len(t, requests, 4, "A enabled: recover with A, despite B having auto-adjust disabled")
				require.Equal(t, "A", requests[2].role)
				require.Contains(t, requests[2].body, "# 本轮只压缩上下文")
				require.Contains(t, requests[2].body, oldHistory)
				require.NotContains(t, requests[2].body, lateHistory, "compression must use the last fitting boundary")
				require.Equal(t, "B", requests[3].role)
				require.Contains(t, requests[3].body, lateHistory)
				if scenario == "compression_success" {
					require.Contains(t, requests[3].body, summary, "B inherits the successfully compressed session history")
					require.NotContains(t, requests[3].body, oldHistory)
				} else {
					require.Contains(t, requests[3].body, oldHistory, "failed compression preserves the original history")
					require.NotContains(t, requests[3].body, summary)
				}
			})
		}
	}
}

func writeModelContextAction(w http.ResponseWriter, native bool, name string, params map[string]any) {
	var delta map[string]any
	finish := "stop"
	if native {
		arguments, _ := json.Marshal(params)
		delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": uuid.NewString(), "type": "function", "function": map[string]any{"name": name, "arguments": string(arguments)}}}}
		finish = "tool_calls"
	} else {
		params["@action"] = name
		content, _ := json.Marshal(params)
		delta = map[string]any{"content": string(content)}
	}
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta, "finish_reason": finish}}})
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
}
