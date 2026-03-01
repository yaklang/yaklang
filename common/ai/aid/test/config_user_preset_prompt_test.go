package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestUserPresetPrompt_Basic(t *testing.T) {
	var capturedPrompt string
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
	longPrompt := strings.Repeat("a", 5000)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt(longPrompt),
	)
	assert.Equal(t, aicommon.UserPresetPromptMaxLength, len(cfg.UserPresetPrompt))
}

func TestUserPresetPrompt_ExactMaxLength(t *testing.T) {
	exactPrompt := strings.Repeat("b", aicommon.UserPresetPromptMaxLength)
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithUserPresetPrompt(exactPrompt),
	)
	assert.Equal(t, aicommon.UserPresetPromptMaxLength, len(cfg.UserPresetPrompt))
	assert.Equal(t, exactPrompt, cfg.UserPresetPrompt)
}

func TestUserPresetPrompt_EmptyNotInjected(t *testing.T) {
	var capturedPrompt string
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
