package aireact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestUIAsyncAnswersPreservePromptCacheLayers(t *testing.T) {
	// Keep all five transport segments visible for these small fixtures, just as
	// the main prompt projection tests do; do not test provider cache billing.
	restore := aiprojection.SetMinCachableUserSegmentBytesForTest(0)
	defer restore()
	for _, form := range []bool{false, true} {
		for _, inherited := range []bool{false, true} {
			name := map[bool]string{false: "questions", true: "form"}[form] + "/" + map[bool]string{false: "root", true: "inherited-runtime"}[inherited]
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				r, err := NewTestReAct(aicommon.WithContext(ctx),
					aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`),
					aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						t.Error("answer admission and prompt assembly must not call a model")
						return nil, nil
					}))
				require.NoError(t, err)
				cfg := r.config
				timeline := cfg.GetTimeline()
				timeline.SetTimelineBucketByteSize(-1)
				_, err = cfg.AppendUserInputHistory("PREVIOUS_USER_REQUEST", time.Now())
				require.NoError(t, err)
				timeline.PushText(cfg.AcquireId(), "PREVIOUS_TOOL_OBSERVATION")
				timeline.FreezeAll()
				frozen := timeline.FreezeSnapshot()
				origin := cfg
				if inherited {
					// Coordinators inherit both the session state and Timeline. A
					// reply must be visible there without appending a second copy.
					origin = aicommon.NewConfig(ctx, aicommon.ConvertConfigToOptionsWithoutHotPatch(cfg)...)
					require.Same(t, cfg.GetSessionPromptState(), origin.GetSessionPromptState())
					require.Same(t, timeline, origin.GetTimeline())
				}
				questions := []interaction.Question{{ID: "scope", Title: "范围", Prompt: "请选择范围", AllowFreeText: true,
					Options: []interaction.Option{{ID: "read", Label: "只读"}, {ID: "write", Label: "修改"}}}}
				const answer = "ASYNC_ANSWER_KEEP_EXACT\n  preserve this indentation"
				var definition *interaction.QuestionForm
				submission := interaction.Submission{ID: "answer-once", Source: "questionnaire",
					Answers: []interaction.Answer{{QuestionID: "scope", Text: answer}}}
				if form {
					questions = nil
					require.NoError(t, json.Unmarshal([]byte(`{"title":"补充参数","schema":{"type":"object","properties":{"scope":{"type":"string"}},"required":["scope"]}}`), &definition))
					submission.Answers = nil
					submission.Values = map[string]any{"scope": answer}
				}
				result, err := origin.RequestClarification(ctx, "origin-task", questions, definition, false, 0)
				require.NoError(t, err)
				submission.RequestID = result.(map[string]any)["requestId"].(string)

				type snapshot struct {
					prompt   string
					messages []aispec.ChatDetail
				}
				capture := func(nonce string) []snapshot {
					t.Helper()
					var snapshots []snapshot
					journal, err := aicommon.MarshalTimeline(timeline)
					require.NoError(t, err)
					for _, native := range []bool{false, true} {
						for _, lightweight := range []bool{false, true} {
							assembled, err := r.AssembleLoopPrompt(nil, &reactloops.LoopPromptAssemblyInput{
								Nonce: nonce, FunctionCallMode: native, Lightweight: lightweight,
								TaskInstruction: "stable task instructions",
							})
							require.NoError(t, err)
							projected := aiprojection.ProjectAndObserve("test-model", assembled.Prompt)
							require.NotNil(t, projected)
							require.Len(t, projected.Messages, 5)
							snapshots = append(snapshots, snapshot{assembled.Prompt, projected.Messages})
						}
					}
					after, err := aicommon.MarshalTimeline(timeline)
					require.NoError(t, err)
					require.JSONEq(t, journal, after, "rendering must not mutate or seal the Timeline")
					return snapshots
				}
				before := capture("before-answer")
				require.NoError(t, cfg.UpdateQuestion(submission.RequestID, "touch"))
				raw, err := json.Marshal(submission)
				require.NoError(t, err)
				for i := 0; i < 2; i++ {
					require.NoError(t, r.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{
						IsFreeInput: true, FreeInput: "UNTRUSTED_CLIENT_TEXT", UserInteraction: string(raw),
					}))
				}
				require.Len(t, cfg.GetUserInputHistory(), 2, "a retried submission must not append another input")
				require.Len(t, origin.GetUserInputHistory(), 2)
				require.Equal(t, frozen, timeline.FreezeSnapshot(), "answer admission must not reopen or seal cached history")
				open := capture("after-answer")
				for i, current := range open {
					require.Equal(t, before[i].messages[:4], current.messages[:4], "all transport messages before the open tail must stay byte-stable")
					require.Contains(t, chatDetailContentString(current.messages[4]), answer)
					require.Contains(t, loopPromptSection(t, current.prompt, "timeline-open"), answer)
					require.Equal(t, 1, strings.Count(current.prompt, answer))
					require.NotContains(t, current.prompt, "UNTRUSTED_CLIENT_TEXT")
					require.NotContains(t, current.prompt, submission.RequestID, "transport metadata is not user history")
				}

				// Sealing may change Semi1. It must move the exact answer out of
				// Open, not duplicate it into Frozen or the dynamic prompt.
				timeline.FreezeAll()
				sealed := capture("after-freeze")
				for i, current := range sealed {
					require.Equal(t, open[i].messages[:2], current.messages[:2], "user-input promotion must not rewrite the static/frozen prefix")
					require.Contains(t, loopPromptSection(t, current.prompt, "semi-dynamic-1"), answer)
					require.NotContains(t, optionalUserInputPromptSection(t, current.prompt, "timeline-open"), answer)
					require.Equal(t, 1, strings.Count(current.prompt, answer))
				}
				journal, err := aicommon.MarshalTimeline(timeline)
				require.NoError(t, err)
				restored, err := aicommon.UnmarshalTimeline(journal)
				require.NoError(t, err)
				options := aicommon.TimelinePromptOptions{PromoteUserInput: true}
				require.Equal(t, aicommon.RenderTimelineFrozenOpenWithOptions(timeline, options),
					aicommon.RenderTimelineFrozenOpenWithOptions(restored, options), "restore must preserve the same layer projection")
			})
		}
	}
}
