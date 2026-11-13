package coordinator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSubForgeClientEventIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delivered []*schema.AiOutputEvent
	session := &Session{Config: aicommon.NewConfig(ctx, aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) { delivered = append(delivered, event) }))}
	task := aicommon.NewStatefulTaskBase("parent-task", "query", ctx, session.GetEmitter(), true)
	call := newSubForgeCall(ctx, session, task, "outer-call", func() {})
	defer call.finish()
	for _, kind := range []schema.EventType{
		schema.EVENT_TYPE_PLAN, schema.EVENT_TYPE_START_PLAN_AND_EXECUTION,
		schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, schema.EVENT_TYPE_FAIL_PLAN_AND_EXECUTION,
		schema.EVENT_TYPE_SUCCESS_REACT, schema.EVENT_TYPE_FAIL_REACT,
		schema.EVENT_TOOL_CALL_DONE, schema.EVENT_TYPE_STRUCTURED,
	} {
		call.handleEvent(&schema.AiOutputEvent{Type: kind, NodeId: "react_task_status_changed", Content: []byte(`{"root_task":{"name":"child"}}`)})
	}
	require.Empty(t, delivered, "child state must not replace parent task or tool state")

	for _, kind := range []schema.EventType{schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE,
		schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
		schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE} {
		t.Run(string(kind), func(t *testing.T) {
			id := string(kind)
			content, err := json.Marshal(map[string]any{"id": id, "selectors": []map[string]string{
				{"value": "continue", "prompt": "Continue"}, {"value": "cancel", "prompt": "Cancel"},
			}, "plans": map[string]any{"root_task": map[string]string{"name": "child"}}})
			require.NoError(t, err)
			original := &schema.AiOutputEvent{Type: kind, IsJson: true, Content: content}
			call.handleEvent(original)
			forwarded := delivered[len(delivered)-1]
			require.Equal(t, kind, original.Type, "presentation must not mutate child persistence")
			require.Equal(t, schema.EventType(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE), forwarded.Type)
			require.Equal(t, id, forwarded.GetInteractiveId())
			require.Equal(t, "outer-call", forwarded.CallToolID)
			require.Equal(t, task.GetId(), forwarded.TaskId)
			require.Equal(t, call.childID, forwarded.CoordinatorId)
			var card struct {
				Prompt  string `json:"prompt"`
				Options []struct {
					Title string `json:"prompt_title"`
				} `json:"options"`
			}
			require.NoError(t, json.Unmarshal(forwarded.Content, &card))
			require.Contains(t, card.Prompt, string(content), "review evidence must remain available")
			require.Equal(t, "continue", card.Options[0].Title)
			require.Equal(t, "cancel", card.Options[1].Title)
			input := &ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: id, InteractiveJSONInput: `{"suggestion":"cancel"}`}
			require.True(t, call.routeInput(input))
			require.Same(t, input, <-call.input.OutputChannel(), "user choice must reach the original endpoint unchanged")
			call.handleEvent(&schema.AiOutputEvent{Type: schema.EVENT_TYPE_REVIEW_RELEASE, IsJson: true, Content: []byte(`{"id":"` + id + `"}`)})
			require.Equal(t, schema.EVENT_TYPE_REVIEW_RELEASE, delivered[len(delivered)-1].Type)
		})
	}
}
