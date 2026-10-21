package aireact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestLoopPromptCompressionModeChangesOnlyDynamicSection(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			r, err := NewTestReAct(aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true))
			require.NoError(t, err)
			input := &aicommon.LoopPromptAssemblyInput{
				Nonce: "same-mode-nonce", FunctionCallMode: native,
				TaskInstruction: "STATIC_TASK_INSTRUCTION", OutputExample: "STATIC_OUTPUT_EXAMPLE", Schema: `{"type":"object"}`,
				FrozenUserContext: "CURRENT_TASK_CONTEXT", PlanStatus: "CURRENT_PLAN_STATUS", TodoSnapshot: "CURRENT_TODO_SNAPSHOT",
				AutoContext: "CURRENT_ASYNC_STATE", ReactiveData: "CURRENT_REACTIVE_STATE", InjectedMemory: "CURRENT_MEMORY",
				TodoCheckpoint: "CURRENT_TODO_CHECKPOINT", ExtraCapabilities: "NORMAL_ONLY_CAPABILITIES",
			}
			normal, err := r.AssembleLoopPrompt(nil, input)
			require.NoError(t, err)
			input.PromptMode = aicommon.LoopPromptModeCompressContext
			input.CompressionGuide = "ONLY_COMPRESS_CONTEXT_WITH_CANDIDATE_IDS"
			compressed, err := r.AssembleLoopPrompt(nil, input)
			require.NoError(t, err)
			require.Equal(t, aicommon.LoopPromptModeNormal, normal.Mode)
			require.Equal(t, aicommon.LoopPromptModeCompressContext, compressed.Mode)
			prefix := func(prompt string) string {
				before, _, found := strings.Cut(prompt, "<|PROMPT_SECTION_dynamic_")
				require.True(t, found)
				return before
			}
			require.Equal(t, prefix(normal.Prompt), prefix(compressed.Prompt), "a mode switch must preserve the entire cacheable prefix and Open task context")
			require.Contains(t, normal.Prompt, "NORMAL_ONLY_CAPABILITIES")
			require.NotContains(t, normal.Prompt, input.CompressionGuide)
			require.NotContains(t, compressed.Prompt, "NORMAL_ONLY_CAPABILITIES")
			for _, material := range []string{input.TaskInstruction, input.FrozenUserContext, input.PlanStatus, input.TodoSnapshot, input.AutoContext, input.ReactiveData, input.InjectedMemory, input.TodoCheckpoint, input.CompressionGuide} {
				require.Contains(t, compressed.Prompt, material)
			}
			section := compressed.Sections[len(compressed.Sections)-1]
			require.Equal(t, "section.dynamic.context_compression", section.Children[0].Key)
			require.Equal(t, input.CompressionGuide, section.Children[0].Content)
		})
	}
}
