package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func promptBuilderChunksBySection(t *testing.T, prompt string) map[string][]*aiprojection.Chunk {
	t.Helper()
	res := aiprojection.Split(prompt)
	require.NotNil(t, res)
	out := make(map[string][]*aiprojection.Chunk)
	for _, c := range res.Chunks {
		out[c.Section] = append(out[c.Section], c)
	}
	return out
}

func TestPromptPrefixBuilder_AssemblePromptWithDynamicSection_DefaultSections(t *testing.T) {
	builder := &PromptPrefixBuilder{
		HighStaticTemplateName:   "high",
		HighStaticTemplate:       "shared-static",
		SemiDynamicTemplateName:  "semi",
		SemiDynamicTemplate:      "{{ .PlanHelp }}",
		SemiDynamicSectionName:   aiprojection.SectionSemiDynamic1,
		SemiDynamic2TemplateName: "semi2",
		SemiDynamic2Template:     "{{ .TaskInstruction }}",
		SemiDynamic2SectionName:  aiprojection.SectionSemiDynamic2,
	}

	prompt, err := builder.AssemblePromptWithDynamicSection(
		&PromptMaterials{
			PlanHelp:        "semi body",
			TaskInstruction: "semi2 body",
		},
		"dynamic",
		"dynamic body",
		nil,
		"n1",
	)
	require.NoError(t, err)

	sections := promptBuilderChunksBySection(t, prompt)
	require.NotEmpty(t, sections[aiprojection.SectionHighStatic])
	require.NotEmpty(t, sections[aiprojection.SectionSemiDynamic1])
	require.NotEmpty(t, sections[aiprojection.SectionSemiDynamic2])
	require.NotEmpty(t, sections[aiprojection.SectionDynamic])
	require.Empty(t, sections[aiprojection.SectionRaw])
}

func TestSharedToolCallModePromptsUseOneConsistentBatchContract(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
	}{
		{
			name:   "high static",
			prompt: SharedPlanAndExecHighStaticTemplate,
		},
		{
			name:   "frozen tool inventory",
			prompt: SharedFrozenBlockTemplate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Contains(t, test.prompt, "默认")
			require.Contains(t, test.prompt, "directly_call_tool_calls")
			require.Contains(t, test.prompt, "tool_require_calls")
			require.Contains(t, test.prompt, "简单无歧义")
			require.Contains(t, test.prompt, "嵌套 wrapper")
			require.NotContains(t, test.prompt, "不要先默认单工具")
			require.NotContains(t, test.prompt, "不得仅为沿用单工具而拆成多轮")
		})
	}
}

func TestPromptPrefixBuilder_AssemblePromptWithDynamicSection_CustomSemiSectionNameAndForcedWrapper(t *testing.T) {
	builder := &PromptPrefixBuilder{
		HighStaticTemplateName:   "high",
		HighStaticTemplate:       "shared-static",
		SemiDynamicTemplateName:  "semi",
		SemiDynamicTemplate:      "",
		SemiDynamicSectionName:   aiprojection.SectionSemiDynamic1,
		ForceSemiDynamicSection:  true,
		SemiDynamic2TemplateName: "semi2",
		SemiDynamic2Template:     "{{ .TaskInstruction }}",
		SemiDynamic2SectionName:  aiprojection.SectionSemiDynamic2,
	}

	prompt, err := builder.AssemblePromptWithDynamicSection(
		&PromptMaterials{
			TaskInstruction: "semi2 body",
		},
		"dynamic",
		"dynamic body",
		nil,
		"n2",
	)
	require.NoError(t, err)
	require.Contains(t, prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")

	sections := promptBuilderChunksBySection(t, prompt)
	require.NotEmpty(t, sections[aiprojection.SectionSemiDynamic1])
	require.NotEmpty(t, sections[aiprojection.SectionSemiDynamic2])
	require.Empty(t, sections[aiprojection.SectionRaw])
}

func TestBuildTaggedPromptSectionsWithSectionNamesAndForce_KeepsEmptySemiWrapper(t *testing.T) {
	prompt := BuildTaggedPromptSectionsWithSectionNamesAndForce(
		"high",
		"",
		"",
		aiprojection.SectionSemiDynamic1,
		true,
		"semi2",
		aiprojection.SectionSemiDynamic2,
		"",
		"dynamic",
		"n3",
	)

	require.Contains(t, prompt, "<|AI_CACHE_SEMI_semi|>")
	require.Contains(t, prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")

	sections := promptBuilderChunksBySection(t, prompt)
	require.NotEmpty(t, sections[aiprojection.SectionSemiDynamic1])
	require.Empty(t, sections[aiprojection.SectionRaw])
}
