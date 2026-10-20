package aireact

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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
