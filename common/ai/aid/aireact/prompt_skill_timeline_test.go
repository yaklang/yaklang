package aireact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestPromptManagerSkillTransferUsesPromotedTimelineWithoutManager(t *testing.T) {
	for _, lightweight := range []bool{false, true} {
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("lightweight=%t/native=%t", lightweight, native), func(t *testing.T) {
				react, err := NewTestReAct()
				require.NoError(t, err)
				require.Nil(t, react.GetCurrentLoop(), "the shared prompt must not require a loop skill manager")
				tl := react.config.GetTimeline()
				tl.SetTimelineBucketByteSize(-1)
				require.True(t, tl.PushSkillFileContent(react.config.AcquireId(), "demo-skill", "SKILL.md", "ACTUAL_PROMPT_SKILL_BODY", aicommon.SkillContentSourceManual, 1, 1, 1))
				input := &reactloops.LoopPromptAssemblyInput{
					Nonce: "skill-transfer", Lightweight: lightweight, FunctionCallMode: native,
					TaskInstruction: "continue the task", Schema: `{"type":"object"}`,
				}
				before, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, loopPromptSection(t, before.Prompt, "timeline-open"), "ACTUAL_PROMPT_SKILL_BODY")
				require.NotContains(t, loopPromptSection(t, before.Prompt, "semi-dynamic-1"), "ACTUAL_PROMPT_SKILL_BODY")

				tl.FreezeAll()
				after, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, loopPromptSection(t, after.Prompt, "semi-dynamic-1"), "ACTUAL_PROMPT_SKILL_BODY")
				require.NotContains(t, after.Prompt, "<|SKILLS_CONTEXT_OPEN_")
				require.Equal(t, 1, strings.Count(after.Prompt, "ACTUAL_PROMPT_SKILL_BODY"))
				sections := mustLoopPromptSections(t, after.Sections)
				var observed string
				for _, section := range sections {
					if section.Key == "section.semi_dynamic_1" {
						for _, child := range section.Children {
							if child.Key == "section.semi_dynamic_1.skills_context" {
								observed = child.Content
							}
						}
					}
				}
				require.Equal(t, aicommon.RenderTimelineFrozenOpen(tl).SkillsContext, observed)

				require.True(t, tl.PushSkillFileContent(react.config.AcquireId(), "demo-skill", "SKILL.md", "NEXT_PROMPT_SKILL_PAGE", aicommon.SkillContentSourceManual, 1, 1, 1))
				pending, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Equal(t, loopPromptSection(t, after.Prompt, "semi-dynamic-1"),
					loopPromptSection(t, pending.Prompt, "semi-dynamic-1"))
				require.Contains(t, loopPromptSection(t, pending.Prompt, "timeline-open"), "NEXT_PROMPT_SKILL_PAGE")
				tl.FreezeAll()
				combined, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				for _, body := range []string{"ACTUAL_PROMPT_SKILL_BODY", "NEXT_PROMPT_SKILL_PAGE"} {
					require.Equal(t, 1, strings.Count(combined.Prompt, body))
					require.Contains(t, loopPromptSection(t, combined.Prompt, "semi-dynamic-1"), body)
				}
			})
		}
	}
}

func TestHelperPromptsUseSharedFrozenSkillMaterialsWithoutManager(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	require.Nil(t, react.GetCurrentLoop())
	freezePromptTestSkill(t, react, "HELPER_PROMOTED_SKILL_BODY")
	verification, _, err := react.promptManager.GenerateVerificationPrompt("task", false, "result")
	require.NoError(t, err)
	answer, _, err := react.promptManager.GenerateDirectlyAnswerPrompt("task", nil)
	require.NoError(t, err)
	for _, prompt := range []string{verification, answer} {
		require.Contains(t, loopPromptSection(t, prompt, "semi-dynamic-1"), "HELPER_PROMOTED_SKILL_BODY")
		require.Equal(t, 1, strings.Count(prompt, "HELPER_PROMOTED_SKILL_BODY"))
	}
}
