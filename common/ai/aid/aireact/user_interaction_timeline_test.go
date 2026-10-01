package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestUserInteractionAnswersEnterTimeline(t *testing.T) {
	for _, interaction := range []string{"clarification", "options", "option-values"} {
		t.Run(interaction, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			answer := aitool.InvokeParams{
				"suggestion": "2", "extra_info": "  EXTRA_REQUIREMENT\n\t保留缩进与空白  \n",
				"selected_values": []string{"source_only"},
			}
			var question aitool.InvokeParams
			var react *ReAct
			var err error
			react, err = NewTestReAct(aicommon.WithContext(ctx), aicommon.WithAgreePolicy(aicommon.AgreePolicyManual),
				aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
					if event.Type != schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE {
						return
					}
					require.NoError(t, json.Unmarshal(event.Content, &question))
					react.config.Feed(question.GetAnyToString("id"), answer)
				}))
			require.NoError(t, err)
			react.config.GetTimeline().SetTimelineBucketByteSize(-1)
			react.SetCurrentTask(aicommon.NewStatefulTaskBase("answer-task", "task query", ctx, react.config.GetEmitter()))
			switch interaction {
			case "clarification":
				require.Contains(t, react.AskForClarification(ctx, "Which scope?", []string{"all files", "source only"}), "EXTRA_REQUIREMENT")
			case "options":
				selected, extra, err := react.RequireUserInteract("Which scope?", []string{"all files", "source only"})
				require.NoError(t, err)
				require.Equal(t, "2", selected)
				require.Equal(t, answer.GetAnyToString("extra_info"), extra)
			case "option-values":
				selected, extra, err := react.RequireUserInteractEx("Which scope?", []*InteractOption{
					{Value: "all", Prompt: "all files"}, {Value: "source_only", Prompt: "source only"},
				})
				require.NoError(t, err)
				require.Equal(t, "2", selected)
				require.Equal(t, answer.GetAnyToString("extra_info"), extra)
			}
			items := react.config.GetTimeline().GetIdToTimelineItem()
			require.Equal(t, 1, items.Len(), "each answer must be journaled once, including its question and options")
			item, ok := items.Get(items.Keys()[0])
			require.True(t, ok)
			recorded, ok := item.GetValue().(*aicommon.UserInteraction)
			require.True(t, ok)
			require.JSONEq(t, string(utils.Jsonify(question)), recorded.SystemPrompt)
			require.JSONEq(t, string(utils.Jsonify(answer)), recorded.UserExtraPrompt)
			for _, frozen := range []bool{false, true} {
				if frozen {
					react.config.GetTimeline().FreezeAll()
				}
				for _, native := range []bool{false, true} {
					for _, lightweight := range []bool{false, true} {
						assembled, err := react.promptManager.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{
							Nonce: "answer", FunctionCallMode: native, Lightweight: lightweight,
						})
						require.NoError(t, err)
						section := "timeline-open"
						if frozen {
							section = "semi-dynamic-1"
						}
						require.Contains(t, r2PromptSection(t, assembled.Prompt, section), recorded.SystemPrompt)
						require.Contains(t, r2PromptSection(t, assembled.Prompt, section), recorded.UserExtraPrompt)
						dynamic := aicommon_testutil.MustExtractAITagBlock(t, assembled.Prompt, "PROMPT_SECTION_dynamic").Body
						require.NotContains(t, dynamic, "EXTRA_REQUIREMENT")
					}
				}
			}
		})
	}
}
