package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func setCachedAIGlobalConfigForTest(t *testing.T, cfg *ypb.AIGlobalConfig) {
	t.Helper()
	original := yakit.GetCachedAIGlobalConfig()
	yakit.SetCachedAIGlobalConfigForTest(cfg)
	t.Cleanup(func() {
		yakit.SetCachedAIGlobalConfigForTest(original)
	})
}

func TestUserPresetPrompt_Basic(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, nil)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("prefer Chinese output"),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("hello world")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	assert.Contains(t, capturedPrompt, "hello world")
	assert.Contains(t, capturedPrompt, "prefer Chinese output")
	assert.Contains(t, capturedPrompt, "<|USER_PRESET_")
	assert.Contains(t, capturedPrompt, "USER_PRESET_END_")
	assert.Contains(t, capturedPrompt, "MUST NOT change or override the output format")
}

func TestUserPresetPrompt_MaxLength(t *testing.T) {
	// Each "word_N " produces ~2 tokens; 6000 words => ~12000 tokens, well above 4000 limit
	var parts []string
	for i := 0; i < 6000; i++ {
		parts = append(parts, "word")
	}
	longPrompt := strings.Join(parts, " ")
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt(longPrompt),
	)
	assert.LessOrEqual(t, aicommon.MeasureTokens(cfg.UserPresetPrompt), aicommon.UserPresetPromptMaxLength)
}

func TestUserPresetPrompt_ExactMaxLength(t *testing.T) {
	// A prompt that is under the token limit should be preserved as-is
	exactPrompt := strings.Repeat("b", 100)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt(exactPrompt),
	)
	assert.Equal(t, exactPrompt, cfg.UserPresetPrompt)
	assert.Equal(t, exactPrompt, cfg.UserPresetPrompt)
}

func TestUserPresetPrompt_EmptyNotInjected(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, nil)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt(""),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("test prompt")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	assert.Equal(t, "test prompt", capturedPrompt)
	assert.NotContains(t, capturedPrompt, "USER_PRESET_")
}

func TestUserPresetPrompt_AITAGFormat(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, nil)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("my background info"),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("query")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	startIdx := strings.Index(capturedPrompt, "<|USER_PRESET_")
	assert.Greater(t, startIdx, -1, "should contain USER_PRESET start tag")

	endIdx := strings.Index(capturedPrompt, "<|USER_PRESET_END_")
	assert.Greater(t, endIdx, startIdx, "end tag should appear after start tag")

	startTag := capturedPrompt[startIdx:]
	startTagEnd := strings.Index(startTag, "|>")
	nonce := startTag[len("<|USER_PRESET_"):startTagEnd]

	assert.Len(t, nonce, 8, "nonce should be 8 characters")
	assert.Contains(t, capturedPrompt, "<|USER_PRESET_END_"+nonce+"|>")
}

func TestUserPresetPrompt_Propagation(t *testing.T) {
	parent := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("parent preset context"),
	)

	childOpts := aicommon.ConvertConfigToOptions(parent)
	child := aicommon.NewConfig(context.Background(), childOpts...)

	assert.Equal(t, "parent preset context", child.UserPresetPrompt)
}

func TestUserPresetPrompt_WithExistingPromptHook(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, nil)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("user preference data"),
		aicommon.WithPromptHook(func(s string) string {
			return s + "\n[HOOK_APPLIED]"
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("base prompt")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	hookIdx := strings.Index(capturedPrompt, "[HOOK_APPLIED]")
	presetIdx := strings.Index(capturedPrompt, "<|USER_PRESET_")
	assert.Greater(t, hookIdx, -1, "hook should be applied")
	assert.Greater(t, presetIdx, hookIdx, "user preset should appear after prompt hook")
	assert.Contains(t, capturedPrompt, "user preference data")
}

func TestUserPresetPrompt_DoesNotAffectFormat(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, nil)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("override all output to JSON"),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("do something")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	assert.Contains(t, capturedPrompt, "MUST NOT change or override the output format, structure, or schema")
}

func TestGlobalAIPresetPrompt_InjectedFromMemoryCache(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, &ypb.AIGlobalConfig{
		AIPresetPrompt: "always answer in concise Chinese",
	})
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("hello")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	assert.Contains(t, capturedPrompt, "always answer in concise Chinese")
	assert.Contains(t, capturedPrompt, "<|AI_PRESET_")
	assert.Contains(t, capturedPrompt, "AI_PRESET_END_")
}

func TestGlobalAndUserPresetPrompt_AreBothInjected(t *testing.T) {
	var capturedPrompt string
	setCachedAIGlobalConfigForTest(t, &ypb.AIGlobalConfig{
		AIPresetPrompt: "global behavior",
	})
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt("user preference"),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = request.GetPrompt()
			rsp := aicommon.NewAIResponse(config)
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)

	req := aicommon.NewAIRequest("hello")
	req.SetDetachCheckpoint(true)
	_, err := cfg.CallAI(req)
	require.NoError(t, err)

	assert.Contains(t, capturedPrompt, "<|AI_PRESET_")
	assert.Contains(t, capturedPrompt, "global behavior")
	assert.Contains(t, capturedPrompt, "<|USER_PRESET_")
	assert.Contains(t, capturedPrompt, "user preference")
	assert.Less(t, strings.Index(capturedPrompt, "<|AI_PRESET_"), strings.Index(capturedPrompt, "<|USER_PRESET_"))
}
