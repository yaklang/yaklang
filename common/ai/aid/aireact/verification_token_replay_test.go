package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type verificationTokenCall struct {
	PromptTokens int     `json:"prompt_tokens"`
	OutputTokens int     `json:"completion_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Seconds      float64 `json:"seconds"`
	Satisfied    bool    `json:"satisfied"`
}

// Opt-in marginal-cost replay: keep primary model actions fixed, use the real
// default/tool/prompt/verifier paths, and send only satisfaction calls to the
// locally configured DeepSeek provider. It measures provider token usage, not
// autonomous model quality. Build this same harness at the baseline and head.
// A warm replay seeds the baseline once, modeling a preceding watchdog check;
// a cold replay does not manufacture that event. Production token gates stay
// unchanged. Reflection permits the same harness after the gate is deleted.
func TestVerificationTokenReplay(t *testing.T) {
	if os.Getenv("YAK_VERIFICATION_TOKEN_LIVE") != "1" {
		t.Skip("set YAK_VERIFICATION_TOKEN_LIVE=1 to spend configured DeepSeek tokens")
	}
	output := os.Getenv("YAK_VERIFICATION_TOKEN_OUTPUT")
	require.NotEmpty(t, output)
	profile := os.Getenv("YAK_VERIFICATION_TOKEN_PROFILE")
	db := consts.GetGormProfileDatabase()
	if path := os.Getenv("YAK_VERIFICATION_TOKEN_CONFIG_DB"); path != "" {
		var err error
		db, err = gorm.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=ro")
		require.NoError(t, err)
		defer db.Close()
	}
	global := &ypb.AIGlobalConfig{}
	if raw := yakit.GetKey(db, consts.AI_GLOBAL_CONFIG_KEY); raw != "" {
		require.NoError(t, json.Unmarshal([]byte(raw), global))
	}
	var selected *ypb.AIModelConfig
	models := append(append([]*ypb.AIModelConfig{}, global.GetLightweightModels()...), global.GetIntelligentModels()...)
	for _, model := range models {
		name := strings.ToLower(model.GetModelName())
		t.Logf("TOKEN_CONFIG tier_model=%s provider=%s", model.GetModelName(), model.GetProvider().GetType())
		if strings.Contains(name, "deepseek") && model.GetProvider() != nil {
			if selected == nil || strings.Contains(name, "flash") {
				selected = model
			}
		}
	}
	if selected == nil {
		if raw := yakit.GetKey(db, consts.GLOBAL_NETWORK_CONFIG); raw != "" {
			network := &ypb.GlobalNetworkConfig{}
			require.NoError(t, json.Unmarshal([]byte(raw), network))
			for _, app := range network.GetAppConfigs() {
				name := ""
				for _, param := range app.GetExtraParams() {
					if param.GetKey() == "model" {
						name = param.GetValue()
					}
				}
				t.Logf("TOKEN_CONFIG legacy_model=%s provider=%s", name, app.GetType())
				if strings.Contains(strings.ToLower(name), "deepseek") || app.GetType() == "deepseek" {
					if name == "" {
						name = "deepseek-chat"
					}
					selected = &ypb.AIModelConfig{Provider: app, ModelName: name}
				}
			}
		}
	}
	require.NotNil(t, selected, "no DeepSeek model found in local AI model configuration")
	provider := selected.GetProvider()
	t.Logf("TOKEN_REPLAY provider=%s model=%s profile=%s", provider.GetType(), selected.GetModelName(), profile)
	providerOptions := []aispec.AIConfigOption{
		aispec.WithType(provider.GetType()), aispec.WithModel(selected.GetModelName()),
		aispec.WithAPIKey(provider.GetAPIKey()), aispec.WithDomain(provider.GetDomain()),
		aispec.WithTimeout(150), aispec.WithDisableStream(true), aispec.WithThinkingLevel("none"),
		func(c *aispec.AIConfig) { c.DisableProviderFallback = true; c.FunctionCallRetryTimes = 1 },
	}
	if profile == "probe" {
		var usage aispec.ChatUsage
		options := append(append([]aispec.AIConfigOption{}, providerOptions...), aispec.WithUsageCallback(func(u *aispec.ChatUsage) {
			if u != nil {
				usage = *u
			}
		}))
		_, err := ai.Chat("只回答 OK。", options...)
		require.NoError(t, err)
		body, _ := json.MarshalIndent(map[string]any{"provider": provider.GetType(), "model": selected.GetModelName(), "usage": usage}, "", "  ")
		require.NoError(t, os.WriteFile(output, body, 0600))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	root := t.TempDir()
	var mu sync.Mutex
	var executed []int
	var calls []verificationTokenCall
	var decisionTokens []int
	decisions, seeded := 0, false
	const batches = 24
	tool, err := aitool.New("verification_token_batch", aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithIntegerParam("batch", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			batch := int(params.GetInt("batch"))
			mu.Lock()
			defer mu.Unlock()
			if batch != len(executed)+1 {
				return nil, fmt.Errorf("out of order batch %d", batch)
			}
			executed = append(executed, batch)
			body, _ := json.Marshal(executed)
			if err := os.WriteFile(filepath.Join(root, "executed.json"), body, 0600); err != nil {
				return nil, err
			}
			var report strings.Builder
			fmt.Fprintf(&report, "SOURCE_BATCH_%02d executed; processed=%d/24. All 24 batches are required before finishing.\n", batch, batch)
			for row := 0; row < 230; row++ {
				fmt.Fprintf(&report, "batch=%02d row=%04d order_id=B%02d-%04d status=paid amount=%d currency=CNY note=independent source observation; retain audit provenance.\n", batch, row, batch, row, batch+row)
			}
			return report.String(), nil
		}))
	require.NoError(t, err)
	var loop *reactloops.ReActLoop
	runtime, err := NewTestReAct(
		aicommon.WithContext(ctx), aicommon.WithWorkdir(root), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithEnableFunctionCallMode(true), aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tool)),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() == "directly-answer" {
				return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": "24 batches processed; see executed.json."}})
			}
			if !strings.HasPrefix(req.GetCallerLabel(), "react-loop:") {
				return nil, fmt.Errorf("unexpected primary call %s", req.GetCallerLabel())
			}
			decisions++
			decisionTokens = append(decisionTokens, aicommon.MeasureTokens(req.GetPrompt()))
			if profile == "warm" && decisions == 1 {
				method := reflect.ValueOf(loop).MethodByName("RefreshVerificationRuntimeSnapshot")
				if method.IsValid() {
					method.Call(nil)
					seeded = true
				}
			}
			if decisions <= batches {
				return guiRespond(c, req, true, guiReplayStep{"directly_call_tool", aitool.InvokeParams{
					"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{"batch": decisions},
					"directly_call_reason": "Process the next required source batch in sequence.",
				}})
			}
			return guiRespond(c, req, true, guiReplayStep{"finish", aitool.InvokeParams{}})
		}),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() != "verification" {
				return nil, fmt.Errorf("unexpected auxiliary call %s", req.GetCallerLabel())
			}
			var usage aispec.ChatUsage
			options := append(append([]aispec.AIConfigOption{}, providerOptions...), aispec.WithContext(req.GetContext()),
				aispec.WithUsageCallback(func(u *aispec.ChatUsage) {
					if u != nil {
						usage = *u
					}
				}))
			began := time.Now()
			text, err := ai.Chat(req.GetPrompt(), options...)
			if err != nil {
				return nil, err
			}
			var verdict map[string]any
			_ = json.Unmarshal([]byte(text), &verdict)
			call := verificationTokenCall{PromptTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens,
				TotalTokens: usage.TotalTokens, Seconds: time.Since(began).Seconds(), Satisfied: verdict["user_satisfied"] == true}
			mu.Lock()
			calls = append(calls, call)
			mu.Unlock()
			t.Logf("TOKEN_REPLAY request=%d prompt=%d completion=%d seconds=%.2f", len(calls), call.PromptTokens, call.OutputTokens, call.Seconds)
			response := c.NewAIResponse()
			response.SetUsageInfo(&usage)
			response.EmitOutputStream(strings.NewReader(text))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)
	defer func() { cancel(); runtime.WaitLifecycleStopped() }()
	runtime.config.RecordRecentlyUsedTool(tool)
	loop, err = reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_DEFAULT, runtime,
		reactloops.BasicAICommonConfigOption(runtime.config)...)
	require.NoError(t, err)
	reactloops.WithPeriodicVerificationInterval(aicommon.DefaultPeriodicVerificationInterval)(loop)
	reactloops.WithDisableLoopPerception(true)(loop)
	reactloops.WithDisableIncreaseIteration(true)(loop)
	reactloops.WithMaxIterations(40)(loop)
	start := time.Now()
	runErr := loop.Execute("verification-token-replay", ctx, "按顺序完成24批本地来源核对。每批都必须实际调用 verification_token_batch，不能省略、合并、重复，全部完成后才能 finish。当前尚未执行的批次必须继续执行。")
	mu.Lock()
	defer mu.Unlock()
	inputTokens, outputTokens := 0, 0
	for _, call := range calls {
		inputTokens += call.PromptTokens
		outputTokens += call.OutputTokens
	}
	primaryEstimate := 0
	for _, count := range decisionTokens {
		primaryEstimate += count
	}
	result := map[string]any{
		"variant": os.Getenv("YAK_VERIFICATION_TOKEN_VARIANT"), "profile": profile, "baseline_commit": "9d21a5b1834d36cf440e8e68883cb8b91aa88749",
		"model": selected.GetModelName(), "provider": provider.GetType(), "thinking": "none",
		"method":               "fixed primary action replay; real default loop/tool/prompt/verifier paths; DeepSeek satisfaction requests metered by provider",
		"warm_baseline_seeded": seeded, "batches_required": batches, "batches_executed": executed, "primary_decisions": decisions,
		"verification_calls": len(calls), "verification_prompt_tokens": inputTokens, "verification_completion_tokens": outputTokens,
		"verification_total_tokens": inputTokens + outputTokens, "primary_prompt_tokens_local_estimate": primaryEstimate,
		"primary_prompt_tokens_each_local_estimate": decisionTokens, "requests": calls, "seconds": time.Since(start).Seconds(),
		"passed":      runErr == nil && len(executed) == batches && decisions == batches+1,
		"limitations": "Primary actions are scripted, not autonomous DeepSeek decisions. Main-prompt token counts are local estimates. Warm profile explicitly models a preceding watchdog-established baseline; cold replay has no two-minute idle window. Cached tokens are not converted to money.",
	}
	if runErr != nil {
		result["error"] = runErr.Error()
	}
	body, _ := json.MarshalIndent(result, "", "  ")
	require.NoError(t, os.WriteFile(output, body, 0600))
	t.Logf("TOKEN_REPLAY_DONE profile=%s batches=%d calls=%d input=%d output=%d seconds=%.2f error=%v", profile, len(executed), len(calls), inputTokens, outputTokens, time.Since(start).Seconds(), runErr)
	require.NoError(t, runErr)
	require.Len(t, executed, batches)
	require.Equal(t, batches+1, decisions)
	if len(calls) > 0 {
		require.Positive(t, inputTokens, "provider must report usage")
	}
}
