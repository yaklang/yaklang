package reactloopstests

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReActLoop_StreamFieldsDoNotAutomaticallyBindPromptReferenceMaterial(t *testing.T) {
	var (
		eventsMu sync.Mutex
		events   []*schema.AiOutputEvent
	)

	reactIns, err := aireact.NewTestReAct(
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, event)
		}),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			response := config.NewAIResponse()
			response.EmitOutputStream(bytes.NewBufferString(`{
				"@action": "capture_summary",
				"human_readable_thought": "Brief status",
				"summary": "Substantive streamed result"
			}`))
			response.Close()
			return response, nil
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("no-automatic-prompt-reference", reactIns,
		reactloops.WithRegisterLoopActionWithStreamField(
			"capture_summary",
			"capture summary stream",
			nil,
			[]*reactloops.LoopStreamField{{FieldName: "summary", AINodeId: "test-summary"}},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				operator.Exit()
			},
		),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("test-task", ctx, "sensitive prompt content"))

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	streamSources := make(map[string]bool)
	for _, event := range events {
		if event == nil {
			continue
		}
		require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, event.Type,
			"stream fields must not automatically expose the decision prompt as reference material")
		if event.Type == schema.EVENT_TYPE_STREAM_START {
			streamSources[event.VizSource] = true
		}
	}

	require.True(t, streamSources["human_readable_thought"])
	require.True(t, streamSources["summary"])
}
