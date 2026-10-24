package aicommon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func clarificationForm(t *testing.T) *interaction.QuestionForm {
	t.Helper()
	var form interaction.QuestionForm
	require.NoError(t, json.Unmarshal([]byte(`{"title":"部署参数","schema":{"type":"object","properties":{"count":{"type":"integer","minimum":1,"maximum":5,"default":2}},"required":["count"]}}`), &form))
	return &form
}

func TestUIFormSharesQuestionLifecycle(t *testing.T) {
	for _, blocking := range []bool{false, true} {
		t.Run(map[bool]string{false: "async", true: "blocking"}[blocking], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			events := make(chan *schema.AiOutputEvent, 32)
			cfg := NewTestConfig(ctx, WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`), WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
			done := make(chan error, 1)
			go func() {
				_, err := cfg.RequestClarification(ctx, "task", nil, clarificationForm(t), blocking, 10)
				done <- err
			}()
			e := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool { return e.Type == "ui_question" })
			var q interaction.QuestionRequest
			require.NoError(t, json.Unmarshal(e.Content, &q))
			require.NotNil(t, q.Form)
			require.Empty(t, q.Questions)
			if blocking {
				require.Zero(t, q.Deadline)
				select {
				case <-done:
					t.Fatal("blocking form returned before answer")
				default:
				}
			} else {
				require.NoError(t, <-done)
				require.Positive(t, q.Deadline)
			}
			require.NoError(t, cfg.UpdateQuestion(q.ID, "touch"))
			require.Zero(t, cfg.UISession().questions[q.ID].Request.Deadline)
			deliveries := 0
			deliver := func(text, task string, origin *Config) (string, error) {
				deliveries++
				require.Equal(t, "对追问的回答：部署参数\ncount：3", text)
				require.Equal(t, "task", task)
				return task, nil
			}
			receipt := interaction.Submission{ID: "once", Source: "questionnaire", RequestID: q.ID, Values: map[string]any{"count": 6}}
			send := func() error {
				raw, _ := json.Marshal(receipt)
				return cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw), FreeInput: "untrusted"}, deliver)
			}
			require.Error(t, send())
			require.Zero(t, deliveries)
			receipt.Values["count"] = 3
			require.NoError(t, send())
			require.NoError(t, send())
			require.Equal(t, 1, deliveries)
			receipt.ID = "another-click"
			require.ErrorContains(t, send(), "already answered")
			if blocking {
				require.NoError(t, <-done)
			}
			_, err := cfg.RequestQuestions(ctx, "task", uiQuestions(), false, 0)
			require.ErrorContains(t, err, "limit")
		})
	}
}

func TestUIFormRuntimePolicyAndDisplayBoundary(t *testing.T) {
	cfg, doc := uiDisplayFixture(t)
	result, err := cfg.RequestClarification(cfg.GetContext(), "task", nil, clarificationForm(t), false, 0)
	require.NoError(t, err)
	require.NoError(t, WithAllowRequireForUserInteract(false)(cfg))
	for _, blocking := range []bool{false, true} {
		_, err = cfg.RequestClarification(cfg.GetContext(), "other", nil, clarificationForm(t), blocking, 0)
		require.ErrorContains(t, err, "disabled")
	}
	raw, _ := json.Marshal(interaction.Submission{ID: "stale", Source: "questionnaire", RequestID: result.(map[string]any)["requestId"].(string), Values: map[string]any{"count": 3}})
	require.ErrorContains(t, cfg.AcceptQuestionAnswer(&ypb.AIInputEvent{UserInteraction: string(raw)}, func(string, string, *Config) (string, error) { t.Fatal("disabled answer delivered"); return "", nil }), "disabled")
	_, err = cfg.RenderUI(cfg.GetContext(), "display", doc)
	require.NoError(t, err)
	for _, enabled := range []bool{true, false} {
		require.NoError(t, WithAllowRequireForUserInteract(enabled)(cfg))
		doc.Nodes[0].Action = &interaction.ComponentAction{Name: "submit"}
		_, err = cfg.RenderUI(cfg.GetContext(), "display", doc)
		require.Error(t, err)
	}
}

func TestUIFormDisablingResumesBlockingWaitWithoutCancellingParent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 32)
	cfg := NewTestConfig(ctx, WithClientUI(`{"protocol":"memfit.ui/1","questions":true}`), WithEventHandler(func(e *schema.AiOutputEvent) { events <- e }))
	done := make(chan any, 1)
	form := clarificationForm(t)
	go func() {
		result, err := cfg.RequestClarification(ctx, "task", nil, form, true, 0)
		if err != nil {
			done <- err
		} else {
			done <- result
		}
	}()
	e := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool { return e.Type == "ui_question" })
	var q interaction.QuestionRequest
	require.NoError(t, json.Unmarshal(e.Content, &q))
	child := NewTestConfig(ctx, ConvertConfigToOptionsWithoutHotPatch(cfg)...)
	require.NoError(t, WithAllowRequireForUserInteract(false)(child))
	select {
	case <-done:
		t.Fatal("worker disabled parent's pending question")
	default:
	}
	require.NoError(t, WithAllowRequireForUserInteract(false)(cfg))
	select {
	case result := <-done:
		require.Equal(t, map[string]any{"status": "cancelled", "answer": ""}, result)
	case <-ctx.Done():
		t.Fatal("disabling questions left a blocked call")
	}
}
