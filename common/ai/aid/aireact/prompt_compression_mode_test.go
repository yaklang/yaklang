package aireact

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestLoopPromptCompressionModeChangesOnlyDynamicSection(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			r, err := NewTestReAct(aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true))
			require.NoError(t, err)
			taskInstruction, outputExample := uuid.NewString(), uuid.NewString()
			taskContext, planStatus := uuid.NewString(), uuid.NewString()
			todoSnapshot, autoContext := uuid.NewString(), uuid.NewString()
			reactiveData, injectedMemory := uuid.NewString(), uuid.NewString()
			todoCheckpoint, extraCapabilities := uuid.NewString(), uuid.NewString()
			compressionGuide := uuid.NewString()
			input := &aicommon.LoopPromptAssemblyInput{
				Nonce: "same-mode-nonce", FunctionCallMode: native,
				TaskInstruction: taskInstruction, OutputExample: outputExample, Schema: `{"type":"object"}`,
				FrozenUserContext: taskContext, PlanStatus: planStatus, TodoSnapshot: todoSnapshot,
				AutoContext: autoContext, ReactiveData: reactiveData, InjectedMemory: injectedMemory,
				TodoCheckpoint: todoCheckpoint, ExtraCapabilities: extraCapabilities,
			}
			normal, err := r.AssembleLoopPrompt(nil, input)
			require.NoError(t, err)
			input.PromptMode = aicommon.LoopPromptModeCompressContext
			input.CompressionGuide = compressionGuide
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
			require.Contains(t, normal.Prompt, extraCapabilities)
			require.NotContains(t, normal.Prompt, input.CompressionGuide)
			require.NotContains(t, compressed.Prompt, extraCapabilities)
			for _, material := range []string{input.TaskInstruction, input.FrozenUserContext, input.PlanStatus, input.TodoSnapshot, input.AutoContext, input.ReactiveData, input.InjectedMemory, input.TodoCheckpoint, input.CompressionGuide} {
				require.Contains(t, compressed.Prompt, material)
			}
			section := compressed.Sections[len(compressed.Sections)-1]
			require.Equal(t, "section.dynamic.context_compression", section.Children[0].Key)
			require.Equal(t, input.CompressionGuide, section.Children[0].Content)
		})
	}
}
