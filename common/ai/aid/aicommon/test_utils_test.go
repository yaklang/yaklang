package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPromptNonce(t *testing.T) {
	t.Run("extracts standard ai tag nonce", func(t *testing.T) {
		prompt := "<|FINAL_ANSWER_ab12|>\nbody\n<|FINAL_ANSWER_END_ab12|>"
		require.Equal(t, "ab12", ExtractPromptNonce(prompt, "FINAL_ANSWER"))
	})

	t.Run("extracts relaxed cache tool nonce", func(t *testing.T) {
		prompt := "<|CACHE_TOOL_CALL_xy99>\nbody"
		require.Equal(t, "xy99", ExtractPromptNonce(prompt, "CACHE_TOOL_CALL"))
	})

	t.Run("extracts legacy angle tag nonce", func(t *testing.T) {
		prompt := "<background_n123>\nbody"
		require.Equal(t, "n123", ExtractPromptNonce(prompt, "background"))
	})
}

func TestExtractDynamicSectionNonce(t *testing.T) {
	prompt := "<|PROMPT_SECTION_high-static|>\nstatic\n<|PROMPT_SECTION_END_high-static|>\n\n<|PROMPT_SECTION_dynamic_n123|>\ndynamic\n<|PROMPT_SECTION_dynamic_END_n123|>"
	require.Equal(t, "n123", ExtractDynamicSectionNonce(prompt))
	require.Equal(t, "n123", ExtractPromptSectionNonce(prompt, "dynamic"))
}
