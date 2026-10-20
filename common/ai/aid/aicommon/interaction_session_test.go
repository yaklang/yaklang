package aicommon

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func uiQuestions() []interaction.Question {
	return []interaction.Question{{ID: "scope", Title: "范围", Prompt: "选择范围", AllowFreeText: true,
		Options: []interaction.Option{{ID: "a", Label: "前端"}, {ID: "b", Label: "前后端", Recommended: true}}}}
}

func TestUIQuestionLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 64)
	c := NewTestConfig(ctx, WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`),
		WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
	result, err := c.RequestQuestions(ctx, "task", uiQuestions(), false, 10)
	require.NoError(t, err)
	id := result.(map[string]any)["requestId"].(string)
	require.NoError(t, c.UpdateQuestion(id, "touch"))
	s := c.UISession()
	s.mu.Lock()
	require.Zero(t, s.questions[id].Request.Deadline)
	require.Equal(t, "b", s.questions[id].Request.Questions[0].Options[0].ID)
	s.mu.Unlock()
	require.NoError(t, c.UpdateQuestion(id, "expire"))
	var deliveries atomic.Int32
	receipt := interaction.Submission{ID: "submit-once", Source: "questionnaire", RequestID: id,
		Answers: []interaction.Answer{{QuestionID: "scope", Selected: []string{"b"}, Text: "只读"}}}
	raw, _ := json.Marshal(receipt)
	event := &ypb.AIInputEvent{UserInteraction: string(raw)}
	deliver := func(text, taskID string, origin *Config) error {
		deliveries.Add(1)
		require.Contains(t, text, "前后端；只读")
		require.Equal(t, "task", taskID)
		require.Same(t, c, origin)
		return nil
	}
	require.NoError(t, c.AcceptQuestionAnswer(event, deliver))
	require.NoError(t, c.AcceptQuestionAnswer(event, deliver))
	require.EqualValues(t, 1, deliveries.Load())
}

func TestUIBlockingQuestionCancellationAndAnswer(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "answer", true: "cancel"}[cancelRequest], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			events := make(chan *schema.AiOutputEvent, 32)
			c := NewTestConfig(ctx, WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`),
				WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
			done := make(chan error, 1)
			go func() { _, err := c.RequestQuestions(ctx, "task", uiQuestions(), true, 1); done <- err }()
			e := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool { return e.Type == "ui_question" })
			var q interaction.QuestionRequest
			require.NoError(t, json.Unmarshal(e.Content, &q))
			require.Zero(t, q.Deadline)
			if cancelRequest {
				require.NoError(t, c.UpdateQuestion(q.ID, "cancel"))
			} else {
				raw, _ := json.Marshal(interaction.Submission{ID: "answer", Source: "questionnaire", RequestID: q.ID,
					Answers: []interaction.Answer{{QuestionID: "scope", Text: "自述答案"}}})
				require.NoError(t, c.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, func(string, string, *Config) error { return nil }))
			}
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("blocking call did not resume")
			}
		})
	}
}

func TestUIExpiredQuestionCanBeAnswered(t *testing.T) {
	c := NewTestConfig(context.Background(), WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`))
	result, err := c.RequestQuestions(c.GetContext(), "old-task", uiQuestions(), false, 0)
	require.NoError(t, err)
	id := result.(map[string]any)["requestId"].(string)
	s := c.UISession()
	s.mu.Lock()
	s.questions[id].Request.Deadline = time.Now().Add(-time.Second).UnixMilli()
	s.mu.Unlock()
	require.NoError(t, c.UpdateQuestion(id, "expire"))
	raw, _ := json.Marshal(interaction.Submission{ID: "late", Source: "questionnaire", RequestID: id,
		Answers: []interaction.Answer{{QuestionID: "scope", Selected: []string{"a"}}}})
	require.NoError(t, c.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, func(string, string, *Config) error { return nil }))
}
