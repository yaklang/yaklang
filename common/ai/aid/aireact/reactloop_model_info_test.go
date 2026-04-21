package aireact

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReActLoop_MainLoopEventsInheritAIModelInfoFromResponse(t *testing.T) {
	expectedService := ksuid.New().String()
	expectedModel := ksuid.New().String()

	var (
		mu          sync.Mutex
		captured    []*schema.AiOutputEvent
		aiCallCount int
	)

	reactIns, err := NewTestReAct(
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e == nil {
				return
			}
			mu.Lock()
			captured = append(captured, cloneAiOutputEventForTest(e))
			mu.Unlock()
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			aiCallCount++

			rsp := i.NewAIResponse()
			rsp.SetModelInfo(expectedService, expectedModel)
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "directly_answer",
  "human_readable_thought": "inspect model propagation in reactloop main loop",
  "answer_payload": "metadata propagated"
}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("main-loop-model-info-test", reactIns)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = loop.Execute("main-loop-model-info-task", ctx, "verify AI metadata on main loop stream nodes")
	require.NoError(t, err)
	loop.GetEmitter().WaitForStream()

	require.Equal(t, 1, aiCallCount, "expected only the main loop AI callback to run")

	mu.Lock()
	defer mu.Unlock()

	assertNodeEventsHaveAIInfo(t, captured, "re-act-loop-thought", expectedService, expectedModel)
	assertNodeEventsHaveAIInfo(t, captured, "re-act-loop-answer-payload", expectedService, expectedModel)
}

func assertNodeEventsHaveAIInfo(t *testing.T, events []*schema.AiOutputEvent, nodeID, expectedService, expectedModel string) {
	t.Helper()

	var matched []*schema.AiOutputEvent
	for _, event := range events {
		if event == nil || event.NodeId != nodeID {
			continue
		}
		if event.Type != schema.EVENT_TYPE_STREAM_START && event.Type != schema.EVENT_TYPE_STREAM {
			continue
		}
		matched = append(matched, event)
	}

	require.NotEmpty(t, matched, "expected streamed events for node %q", nodeID)
	for _, event := range matched {
		require.Equal(t, expectedService, event.AIService, "unexpected AI service on node %q", nodeID)
		require.Equal(t, expectedModel, event.AIModelName, "unexpected AI model on node %q", nodeID)
		require.NotEmpty(t, event.AIModelVerboseName, "expected verbose model name on node %q", nodeID)
	}
}

func assertNodeStreamContains(t *testing.T, events []*schema.AiOutputEvent, nodeID, expected string) {
	t.Helper()

	var out bytes.Buffer
	for _, event := range events {
		if event == nil || event.NodeId != nodeID || event.Type != schema.EVENT_TYPE_STREAM {
			continue
		}
		out.Write(event.StreamDelta)
	}

	require.Contains(t, out.String(), expected, "expected node %q stream output to contain payload", nodeID)
}

func cloneAiOutputEventForTest(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	if event == nil {
		return nil
	}

	cloned := *event
	if event.Content != nil {
		cloned.Content = append([]byte(nil), event.Content...)
	}
	if event.StreamDelta != nil {
		cloned.StreamDelta = append([]byte(nil), event.StreamDelta...)
	}
	if event.ProcessesId != nil {
		cloned.ProcessesId = append([]string(nil), event.ProcessesId...)
	}
	return &cloned
}
