package aireact

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestUserInputHistoryPromptPromotionAcrossModes(t *testing.T) {
	for _, functionCall := range []bool{false, true} {
		for _, lightweight := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "text", true: "functioncall"}[functionCall], map[bool]string{false: "full", true: "lightweight"}[lightweight]}, "/"), func(t *testing.T) {
				react, err := NewTestReAct()
				require.NoError(t, err)
				react.config.GetTimeline().SetTimelineBucketByteSize(-1)
				original := "  ORIGINAL_USER_INPUT\n\t完整内容与空白  \n" + strings.Repeat("长历史原文不得裁剪\n", 2048)
				_, err = react.config.AppendUserInputHistory(original, time.Now())
				require.NoError(t, err)
				input := &reactloops.LoopPromptAssemblyInput{Nonce: "input-1", UserQuery: "CURRENT_QUERY", FunctionCallMode: functionCall, Lightweight: lightweight}
				open, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, r2PromptSection(t, open.Prompt, "timeline-open"), original)
				require.NotContains(t, r2PromptSection(t, open.Prompt, "semi-dynamic-1"), original)
				require.Contains(t, open.Prompt, "CURRENT_QUERY")
				require.Contains(t, open.Prompt, "用户输入边界")
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

func TestUserInputPromotionIsOptInForMainContext(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	react.config.GetTimeline().SetTimelineBucketByteSize(-1)
	original := "HISTORY_ONLY_USER_INPUT"
	_, err = react.config.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("compat-task", "CURRENT_HELPER_QUERY", react.config.GetContext(), react.config.GetEmitter())
	tool := aitool.NewWithoutCallback("read_file", aitool.WithStringParam("path"))
	for _, sealed := range []bool{false, true} {
		if sealed {
			react.config.GetTimeline().FreezeAll()
		}
		rawBefore, err := aicommon.MarshalTimeline(react.config.GetTimeline())
		require.NoError(t, err)
		legacy := aicommon.BuildPromptFrozenOpenMaterialsWithOptions(react.config, aicommon.TimelinePromptOptions{ExcludeToolCache: true})
		require.False(t, legacy.PromoteUserInput)
		require.Empty(t, legacy.PromptedUserInputHistory)
		if sealed {
			require.Contains(t, legacy.TimelineFrozen, original)
		} else {
			require.Contains(t, legacy.TimelineOpen, original)
		}
		base, err := react.promptManager.GetLoopPromptBaseMaterials(nil, "compat")
		require.NoError(t, err)
		require.Equal(t, react.promptManager.UserHistoryContextWithNonce("compat"), base.UserHistory)
		main, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "main", UserQuery: task.GetUserInput()})
		require.NoError(t, err)
		require.Contains(t, main.Prompt, "用户输入边界")
		require.Contains(t, main.Prompt, react.config.GetTimeline().WrapUserInputForPrompt(task.GetUserInput()))
		native, err := react.promptManager.GenerateFunctionCallToolParamsPromptForTask(task, tool, aicommon.ToolParamsCallIntent{})
		require.NoError(t, err)
		text, err := react.promptManager.GenerateToolParamsPromptWithMetaForTask(task, tool)
		require.NoError(t, err)
		retry, err := react.promptManager.GenerateReGenerateToolParamsPromptWithMeta(task.GetUserInput(), aitool.InvokeParams{"path": "old"}, tool)
		require.NoError(t, err)
		for _, helper := range []string{native, text.Prompt, retry.Prompt} {
			require.Contains(t, helper, original)
			require.Contains(t, helper, task.GetUserInput())
			require.NotContains(t, helper, "PromptedUserInputHistory")
			require.NotContains(t, helper, "USER_INTERACT_")
			require.NotContains(t, helper, "用户输入边界")
		}
		rawAfter, err := aicommon.MarshalTimeline(react.config.GetTimeline())
		require.NoError(t, err)
		require.JSONEq(t, rawBefore, rawAfter, "main and helper rendering must not mutate shared history")
	}
}
