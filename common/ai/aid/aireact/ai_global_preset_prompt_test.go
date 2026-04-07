package aireact

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_GlobalAIPresetPromptFromMemoryConfig(t *testing.T) {
	original := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() {
		yakit.SetCachedAIGlobalConfigForTest(original)
	})

	presetToken := uuid.NewString()
	inputToken := uuid.NewString()
	yakit.SetCachedAIGlobalConfigForTest(&ypb.AIGlobalConfig{
		AIPresetPrompt: presetToken,
	})

	var capturedPrompt string
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = req.GetPrompt()
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	req := aicommon.NewAIRequest(inputToken)
	req.SetDetachCheckpoint(true)
	_, err = ins.config.CallAI(req)
	require.NoError(t, err)

	require.Contains(t, capturedPrompt, inputToken)
	require.Contains(t, capturedPrompt, presetToken)
	require.Contains(t, capturedPrompt, "<|AI_PRESET_")
	require.Contains(t, capturedPrompt, "AI_PRESET_END_")
	require.Contains(t, capturedPrompt, "MUST NOT change or override the output format, structure, or schema")
}

func TestReAct_GlobalAndUserPresetPromptTogether(t *testing.T) {
	original := yakit.GetCachedAIGlobalConfig()
	t.Cleanup(func() {
		yakit.SetCachedAIGlobalConfigForTest(original)
	})

	globalToken := uuid.NewString()
	userToken := uuid.NewString()
	inputToken := uuid.NewString()
	yakit.SetCachedAIGlobalConfigForTest(&ypb.AIGlobalConfig{
		AIPresetPrompt: globalToken,
	})

	var capturedPrompt string
	ins, err := NewTestReAct(
		aicommon.WithUserPresetPrompt(userToken),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			capturedPrompt = req.GetPrompt()
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(strings.NewReader("ok"))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	req := aicommon.NewAIRequest(inputToken)
	req.SetDetachCheckpoint(true)
	_, err = ins.config.CallAI(req)
	require.NoError(t, err)

	require.Contains(t, capturedPrompt, inputToken)
	require.Contains(t, capturedPrompt, globalToken)
	require.Contains(t, capturedPrompt, userToken)
	require.Contains(t, capturedPrompt, "<|AI_PRESET_")
	require.Contains(t, capturedPrompt, "<|USER_PRESET_")
	require.Less(t, strings.Index(capturedPrompt, "<|AI_PRESET_"), strings.Index(capturedPrompt, "<|USER_PRESET_"))
}
