package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicache"
)

func promptBuilderChunksBySection(t *testing.T, prompt string) map[string][]*aicache.Chunk {
	t.Helper()
	res := aicache.Split(prompt)
	require.NotNil(t, res)
	out := make(map[string][]*aicache.Chunk)
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
		SemiDynamicSectionName:   aicache.SectionSemiDynamic1,
		SemiDynamic2TemplateName: "semi2",
		SemiDynamic2Template:     "{{ .TaskInstruction }}",
		SemiDynamic2SectionName:  aicache.SectionSemiDynamic2,
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
	require.NotEmpty(t, sections[aicache.SectionHighStatic])
	require.NotEmpty(t, sections[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sections[aicache.SectionSemiDynamic2])
	require.NotEmpty(t, sections[aicache.SectionDynamic])
	require.Empty(t, sections[aicache.SectionRaw])
}

func TestSharedToolCallModePromptsPreferReadyIndependentBatch(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		decision string
	}{
		{
			name:     "high static",
			prompt:   SharedPlanAndExecHighStaticTemplate,
			decision: "先枚举本轮已经明确、可立即执行的真实工具调用",
		},
		{
			name:     "frozen tool inventory",
			prompt:   SharedFrozenBlockTemplate,
			decision: "先枚举本轮已经明确、可立即执行的真实调用",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Contains(t, test.prompt, test.decision)
			require.Contains(t, test.prompt, "优先")
			require.Contains(t, test.prompt, "不得仅为沿用单工具而拆成多轮")
			require.Contains(t, test.prompt, "任务属于探索阶段")
			require.NotContains(t, test.prompt, "默认单步")
			require.NotContains(t, test.prompt, "探索 / 上游不确定 / 需要逐步收紧时的默认形态")
		})
	}
}

func TestPromptPrefixBuilder_AssemblePromptWithDynamicSection_CustomSemiSectionNameAndForcedWrapper(t *testing.T) {
	builder := &PromptPrefixBuilder{
		HighStaticTemplateName:   "high",
		HighStaticTemplate:       "shared-static",
		SemiDynamicTemplateName:  "semi",
		SemiDynamicTemplate:      "",
		SemiDynamicSectionName:   aicache.SectionSemiDynamic1,
		ForceSemiDynamicSection:  true,
		SemiDynamic2TemplateName: "semi2",
		SemiDynamic2Template:     "{{ .TaskInstruction }}",
		SemiDynamic2SectionName:  aicache.SectionSemiDynamic2,
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
	require.NotEmpty(t, sections[aicache.SectionSemiDynamic1])
	require.NotEmpty(t, sections[aicache.SectionSemiDynamic2])
	require.Empty(t, sections[aicache.SectionRaw])
}

func TestBuildTaggedPromptSectionsWithSectionNamesAndForce_KeepsEmptySemiWrapper(t *testing.T) {
	prompt := BuildTaggedPromptSectionsWithSectionNamesAndForce(
		"high",
		"",
		"",
		aicache.SectionSemiDynamic1,
		true,
		"semi2",
		aicache.SectionSemiDynamic2,
		"",
		"dynamic",
		"n3",
	)

	require.Contains(t, prompt, "<|AI_CACHE_SEMI_semi|>")
	require.Contains(t, prompt, "<|PROMPT_SECTION_semi-dynamic-1|>")

	sections := promptBuilderChunksBySection(t, prompt)
	require.NotEmpty(t, sections[aicache.SectionSemiDynamic1])
	require.Empty(t, sections[aicache.SectionRaw])
}
