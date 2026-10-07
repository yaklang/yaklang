package aireact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestUserInputAttachmentQueryStaysOutOfMainDynamic(t *testing.T) {
	const query = "ATTACHMENT_TASK_QUERY"
	const fileBody = "User Prompt: legitimate attachment text\nFILE_ATTACHMENT_CONTENT"
	path := filepath.Join(t.TempDir(), "attachment.txt")
	require.NoError(t, os.WriteFile(path, []byte(fileBody), 0o600))
	react, err := NewTestReAct()
	require.NoError(t, err)
	react.config.GetTimeline().SetTimelineBucketByteSize(-1)
	react.config.ContextProviderManager.RegisterTracedContent("file", aicommon.FileContextProvider(path, query))
	task := aicommon.NewStatefulTaskBase("attachment-task", query, react.config.GetContext(), react.config.GetEmitter())
	task.SetAttachedDatas([]*aicommon.AttachedResource{{
		Type: aicommon.CONTEXT_PROVIDER_TYPE_FILE, Key: aicommon.CONTEXT_PROVIDER_KEY_FILE_CONTENT, Value: "INLINE_ATTACHMENT_CONTENT",
	}})
	t.Cleanup(installTaskInlineAttachmentProvider(react.config.ContextProviderManager, task))
	_, err = react.config.AppendUserInputHistory(query, time.Now())
	require.NoError(t, err)
	for _, frozen := range []bool{false, true} {
		if frozen {
			react.config.GetTimeline().FreezeAll()
		}
		for _, native := range []bool{false, true} {
			for _, lightweight := range []bool{false, true} {
				// Helpers retain the original query header, even when interleaved
				// with main prompt assembly using the same traced attachment.
				base, err := react.promptManager.GetLoopPromptBaseMaterials(nil, "helper")
				require.NoError(t, err)
				require.Contains(t, base.AutoContext, "User Prompt: "+query)
				assembled, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{
					Nonce: "attachment", FunctionCallMode: native, Lightweight: lightweight,
				})
				require.NoError(t, err)
				require.Equal(t, 1, strings.Count(assembled.Prompt, query))
				dynamic := aicommon_testutil.MustExtractAITagBlock(t, assembled.Prompt, "PROMPT_SECTION_dynamic").Body
				require.NotContains(t, dynamic, query)
				if !lightweight {
					require.Contains(t, dynamic, fileBody, "attachment content must not be filtered by text matching")
					require.Contains(t, dynamic, path)
					require.Contains(t, dynamic, "INLINE_ATTACHMENT_CONTENT")
				}
			}
		}
	}
}

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
				current := "  CURRENT_QUERY\n\t保留当前输入原文  \n" + strings.Repeat("当前 Query 不得裁剪\n", 2048)
				_, err = react.config.AppendUserInputHistory(current, time.Now())
				require.NoError(t, err)
				before, err := aicommon.MarshalTimeline(react.config.GetTimeline())
				require.NoError(t, err)
				input := &reactloops.LoopPromptAssemblyInput{Nonce: "input-1", UserQuery: "UNJOURNALED_HELPER_QUERY", FunctionCallMode: functionCall, Lightweight: lightweight}
				open, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, loopPromptSection(t, open.Prompt, "timeline-open"), original)
				require.NotContains(t, loopPromptSection(t, open.Prompt, "semi-dynamic-1"), original)
				require.Contains(t, loopPromptSection(t, open.Prompt, "timeline-open"), current)
				require.Equal(t, 1, strings.Count(open.Prompt, current))
				require.NotContains(t, open.Prompt, "CURRENT_TASK_INPUT")
				require.NotContains(t, open.Prompt, "USER_QUERY")
				require.NotContains(t, open.Prompt, "UNJOURNALED_HELPER_QUERY", "shared helper query must not enter main context")
				require.NotContains(t, aicommon_testutil.MustExtractAITagBlock(t, open.Prompt, "PROMPT_SECTION_dynamic").Body, "CURRENT_QUERY")
				require.Contains(t, open.Prompt, "关注 Timeline 中的用户输入")
				after, err := aicommon.MarshalTimeline(react.config.GetTimeline())
				require.NoError(t, err)
				require.JSONEq(t, before, after, "main prompt assembly must not mutate Timeline")
				react.config.GetTimeline().FreezeAll()
				input.Nonce = "input-2"
				sealed, err := react.promptManager.AssembleLoopPrompt(nil, input)
				require.NoError(t, err)
				require.Contains(t, loopPromptSection(t, sealed.Prompt, "semi-dynamic-1"), original)
				require.NotContains(t, optionalUserInputPromptSection(t, sealed.Prompt, "timeline-open"), original)
				require.NotContains(t, sealed.Prompt[:strings.Index(sealed.Prompt, loopPromptSection(t, sealed.Prompt, "semi-dynamic-1"))], original)
				require.Contains(t, loopPromptSection(t, sealed.Prompt, "semi-dynamic-1"), current)
				require.NotContains(t, optionalUserInputPromptSection(t, sealed.Prompt, "timeline-open"), current)
				require.Equal(t, 1, strings.Count(sealed.Prompt, current))
				require.NotContains(t, sealed.Prompt, "USER_QUERY")
				require.NotContains(t, sealed.Prompt, "UNJOURNALED_HELPER_QUERY")
				require.NotContains(t, aicommon_testutil.MustExtractAITagBlock(t, sealed.Prompt, "PROMPT_SECTION_dynamic").Body, "CURRENT_QUERY")
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
				require.Equal(t, loopPromptSection(t, sealed.Prompt, "semi-dynamic-1"), loopPromptSection(t, pending.Prompt, "semi-dynamic-1"))
				require.Contains(t, loopPromptSection(t, pending.Prompt, "timeline-open"), "FOLLOWUP_INPUT")
				require.NotContains(t, loopPromptSection(t, pending.Prompt, "semi-dynamic-1"), "FOLLOWUP_INPUT")
			})
		}
	}
}

func optionalUserInputPromptSection(t *testing.T, prompt, name string) string {
	t.Helper()
	if !strings.Contains(prompt, aiprojection.CreateTemplate("<|PROMPT_SECTION_"+name+"|>")) {
		return ""
	}
	return loopPromptSection(t, prompt, name)
}

func TestUserInputPromotionIsOptInForMainContext(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	react.config.GetTimeline().SetTimelineBucketByteSize(-1)
	original := "HISTORY_ONLY_USER_INPUT"
	_, err = react.config.AppendUserInputHistory(original, time.Now())
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("compat-task", "CURRENT_HELPER_QUERY", react.config.GetContext(), react.config.GetEmitter())
	react.config.GetTimeline().EnsureTaskUserInput(task.GetId(), task.GetUserInput(), react.config.AcquireId)
	for _, sealed := range []bool{false, true} {
		if sealed {
			react.config.GetTimeline().FreezeAll()
		}
		rawBefore, err := aicommon.MarshalTimeline(react.config.GetTimeline())
		require.NoError(t, err)
		legacy := aicommon.BuildPromptFrozenOpenMaterialsWithOptions(react.config, aicommon.TimelinePromptOptions{ExcludeToolCache: true})
		require.Empty(t, legacy.PromotedUserInputHistory)
		if sealed {
			require.Contains(t, legacy.TimelineFrozen, original)
		} else {
			require.Contains(t, legacy.TimelineOpen, original)
		}
		base, err := react.promptManager.GetLoopPromptBaseMaterials(nil, "compat")
		require.NoError(t, err)
		require.Equal(t, react.promptManager.UserHistoryContextWithNonce("compat"), base.UserHistory)
		main, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{Nonce: "main"})
		require.NoError(t, err)
		require.Contains(t, main.Prompt, "关注 Timeline 中的用户输入")
		require.Contains(t, main.Prompt, task.GetUserInput())
		require.NotContains(t, main.Prompt, "USER_QUERY")
		rawAfter, err := aicommon.MarshalTimeline(react.config.GetTimeline())
		require.NoError(t, err)
		require.JSONEq(t, rawBefore, rawAfter, "main and helper rendering must not mutate shared history")
	}
}
