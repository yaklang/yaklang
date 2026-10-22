package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestUIQuestionActionRegistrationHonorsSessionPolicy(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[allow], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			react, err := NewTestReAct(aicommon.WithContext(ctx),
				aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`),
				aicommon.WithAllowRequireForUserInteract(allow),
				aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					t.Error("registering actions must not call a model")
					return nil, nil
				}))
			require.NoError(t, err)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, react)
			require.NoError(t, err)
			for _, name := range []string{"ask_for_clarification_async", "ask_for_clarification"} {
				_, err := loop.GetActionHandler(name)
				if allow {
					require.NoError(t, err, name)
				} else {
					require.Error(t, err, name)
				}
			}
		})
	}
}

func TestUIAnswersSteerRunningTaskAndQueueLateReplies(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(map[bool]string{true: "active", false: "late"}[active], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := aicommon.NewConfig(ctx, aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`),
				aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) { return nil, nil }))
			r := &ReAct{config: cfg, Emitter: cfg.Emitter, taskQueue: NewTaskQueue("ui-test")}
			task := aicommon.NewStatefulTaskBase("origin-task", "original input", ctx, cfg.Emitter)
			defer task.Cancel()
			if active {
				task.SetStatus(aicommon.AITaskState_Processing)
				r.AddRuntimeTask(task)
			}
			result, err := cfg.RequestQuestions(task.GetContext(), task.GetId(), []interaction.Question{{
				ID: "scope", Title: "范围", Prompt: "选择范围", AllowFreeText: true,
				Options: []interaction.Option{{ID: "a", Label: "只读"}, {ID: "b", Label: "修改"}},
			}}, false, 0)
			require.NoError(t, err)
			if !active {
				task.Cancel()
			}
			id := result.(map[string]any)["requestId"].(string)
			raw, _ := json.Marshal(interaction.Submission{ID: "submit", Source: "questionnaire", RequestID: id,
				Answers: []interaction.Answer{{QuestionID: "scope", Selected: []string{"a"}}}})
			wakes := 0
			cfg.InputEventManager.RegisterAfterInputEvent("test-wakeup", func(event *ypb.AIInputEvent) {
				if event.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
					wakes++
				}
			})
			for i := 0; i < 2; i++ {
				require.NoError(t, r.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{IsFreeInput: true, UserInteraction: string(raw), FreeInput: "untrusted client text"}))
			}
			if active {
				require.Equal(t, 1, wakes)
				require.True(t, r.taskQueue.IsEmpty())
				require.Len(t, cfg.GetUserInputHistory(), 1)
				require.Contains(t, cfg.GetUserInputHistory()[0].UserInput, "只读")
				require.NotContains(t, cfg.GetUserInputHistory()[0].UserInput, "untrusted")
			} else {
				require.Zero(t, wakes)
				require.Equal(t, 1, r.taskQueue.GetQueueingCount())
				require.Empty(t, cfg.GetUserInputHistory(), "the queue persists ordinary user history at dequeue")
				require.Contains(t, r.taskQueue.PeekFirst().GetUserInput(), "只读")
			}
		})
	}
}

func TestClarificationGoalAndSubagentVisibility(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithEnableGoalMode(true), aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`))
	require.NoError(t, err)
	loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r)
	require.NoError(t, err)
	require.Contains(t, loop.GetAllActionNames(), "ask_for_clarification_async")
	require.NotContains(t, loop.GetAllActionNames(), "ask_for_clarification")
	require.NotContains(t, loop.GetAllActionNames(), "request_user_input")
	loop.Set(reactloops.SubAgentDepthLoopVar, 1)
	require.NotContains(t, loop.GetAllActionNames(), "ask_for_clarification_async")
	_, err = loop.GetActionHandler("ask_for_clarification_async")
	require.Error(t, err)
}

func TestLegacyClarificationUsesRichLifecycleWhenSupported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan interaction.QuestionRequest, 1)
	r, err := NewTestReAct(aicommon.WithContext(ctx), aicommon.WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.Type == "ui_question" {
			var q interaction.QuestionRequest
			if json.Unmarshal(e.Content, &q) == nil && q.State == "pending" {
				requests <- q
			}
		}
	}))
	require.NoError(t, err)
	done := make(chan string, 1)
	go func() { done <- r.AskForClarification(ctx, "请选择范围", []string{"前端", "后端"}) }()
	var q interaction.QuestionRequest
	select {
	case q = <-requests:
	case <-time.After(3 * time.Second):
		t.Fatal("rich question was not emitted")
	}
	require.Equal(t, "blocking", q.Mode)
	raw, _ := json.Marshal(interaction.Submission{ID: "legacy-answer", Source: "questionnaire", RequestID: q.ID, Answers: []interaction.Answer{{QuestionID: "clarification", Selected: []string{"2"}}}})
	cfg := r.GetConfig().(*aicommon.Config)
	require.NoError(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, func(string, string, *aicommon.Config) (string, error) { return "", nil }))
	select {
	case answer := <-done:
		require.Contains(t, answer, "后端")
	case <-time.After(3 * time.Second):
		t.Fatal("blocking call did not resume")
	}
	require.ErrorContains(t, cfg.CheckClarificationAllowed("", false), "limit")
}
