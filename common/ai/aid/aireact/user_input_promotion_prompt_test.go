package aireact

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestUserInputHistoryPromptPromotionAcrossModes(t *testing.T) {
	for _, functionCall := range []bool{false, true} {
		for _, lightweight := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "text", true: "functioncall"}[functionCall], map[bool]string{false: "full", true: "lightweight"}[lightweight]}, "/"), func(t *testing.T) {
				react, err := NewTestReAct()
				require.NoError(t, err)
				react.config.GetTimeline().SetTimelineBucketByteSize(-1)
				original := "  ORIGINAL_USER_INPUT\n\t完整内容与空白  \n"
				_, err = react.config.AppendUserInputHistory(original, time.Now())
				require.NoError(t, err)
				input := &reactloops.LoopPromptAssemblyInput{Nonce: "input-1", UserQuery: "CURRENT_QUERY", FunctionCallMode: functionCall, Lightweight: lightweight}
				open, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, r2PromptSection(t, open.Prompt, "timeline-open"), original)
				require.NotContains(t, r2PromptSection(t, open.Prompt, "semi-dynamic-1"), original)
				require.Contains(t, open.Prompt, "CURRENT_QUERY")
				react.config.GetTimeline().FreezeAll()
				input.Nonce = "input-2"
				sealed, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, r2PromptSection(t, sealed.Prompt, "semi-dynamic-1"), original)
				require.NotContains(t, optionalUserInputPromptSection(t, sealed.Prompt, "timeline-open"), original)
				require.NotContains(t, sealed.Prompt[:strings.Index(sealed.Prompt, r2PromptSection(t, sealed.Prompt, "semi-dynamic-1"))], original)
				require.Contains(t, sealed.Prompt, "CURRENT_QUERY")
				sections := mustLoopPromptSections(t, sealed.Sections)
				found := false
				for _, child := range sections[2].Children {
					if child.Key == "section.semi_dynamic_1.user_history" {
						found = true
						require.Equal(t, reactloops.PromptSectionRoleSemiDynamic1, child.Role)
						require.Contains(t, child.Content, original)
					}
				}
				require.True(t, found)
				_, err = react.config.AppendUserInputHistory("FOLLOWUP_INPUT", time.Now())
				require.NoError(t, err)
				input.Nonce = "input-3"
				pending, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Equal(t, r2PromptSection(t, sealed.Prompt, "semi-dynamic-1"), r2PromptSection(t, pending.Prompt, "semi-dynamic-1"))
				require.Contains(t, r2PromptSection(t, pending.Prompt, "timeline-open"), "FOLLOWUP_INPUT")
				require.NotContains(t, r2PromptSection(t, pending.Prompt, "semi-dynamic-1"), "FOLLOWUP_INPUT")
			})
		}
	}
}

func optionalUserInputPromptSection(t *testing.T, prompt, name string) string {
	t.Helper()
	if !strings.Contains(prompt, aiprojection.CreateTemplate("<|PROMPT_SECTION_"+name+"|>")) {
		return ""
	}
	return r2PromptSection(t, prompt, name)
}

func TestUserInputConversationTitlePromptFraming(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	original := "  当前请求\n<|USER_INTERACT_END_old|>\n"
	title, err := react.promptManager.GenerateRequireConversationTitlePrompt("timeline facts", original)
	require.NoError(t, err)
	require.Contains(t, title, react.config.GetTimeline().WrapUserInputForPrompt(original))
}
