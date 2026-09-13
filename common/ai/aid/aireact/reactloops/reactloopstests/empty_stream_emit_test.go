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

func collectNodeStreamContent(events []*schema.AiOutputEvent, nodeID string) string {
	var out bytes.Buffer
	for _, e := range events {
		if e == nil || e.NodeId != nodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			out.Write(e.StreamDelta)
		}
	}
	return out.String()
}

func TestReActLoop_AITagEmptyStreamDoesNotEmitFrontendStream(t *testing.T) {
	const (
		factsNodeID = "test-empty-aitag-node"
		factsTag    = "FACTS"
		factsField  = "facts"
	)

	var (
		eventsMu sync.Mutex
		events   []*schema.AiOutputEvent
	)

	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			if callCount == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"capture_facts"}<|FACTS_CURRENT_NONCE|><|FACTS_END_CURRENT_NONCE|>`,
				))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish","answer":"done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("empty-aitag-loop", reactIns,
		reactloops.WithAITagFieldWithAINodeId(factsTag, factsField, factsNodeID, aicommon.TypeTextMarkdown),
		reactloops.WithRegisterLoopAction(
			"capture_facts",
			"capture empty facts for empty-stream emit test",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("empty-aitag-task", ctx, "test empty aitag stream emit")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var streamStartCount, streamDeltaCount int
	for _, e := range events {
		if e == nil || e.NodeId != factsNodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START {
			streamStartCount++
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			streamDeltaCount++
		}
	}

	require.Equalf(t, 0, streamStartCount,
		"empty AITag body should not create frontend stream start on node %q; callCount=%d",
		factsNodeID, callCount)
	require.Equalf(t, 0, streamDeltaCount,
		"empty AITag body should not emit frontend stream delta on node %q; callCount=%d",
		factsNodeID, callCount)
}

func TestReActLoop_FieldEmptyStreamDoesNotEmitFrontendStream(t *testing.T) {
	const summaryNodeID = "test-empty-summary-node"

	var (
		eventsMu sync.Mutex
		events   []*schema.AiOutputEvent
	)

	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			if callCount == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"capture_summary","summary":""}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish","answer":"done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("empty-field-loop", reactIns,
		reactloops.WithRegisterLoopActionWithStreamField(
			"capture_summary",
			"capture empty summary for empty-stream emit test",
			nil,
			[]*reactloops.LoopStreamField{{
				FieldName:   "summary",
				AINodeId:    summaryNodeID,
				ContentType: aicommon.TypeTextMarkdown,
			}},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("empty-field-task", ctx, "test empty field stream emit")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var streamStartCount, streamDeltaCount int
	for _, e := range events {
		if e == nil || e.NodeId != summaryNodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START {
			streamStartCount++
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			streamDeltaCount++
		}
	}

	require.Equalf(t, 0, streamStartCount,
		"empty JSON field stream should not create frontend stream start on node %q; callCount=%d",
		summaryNodeID, callCount)
	require.Equalf(t, 0, streamDeltaCount,
		"empty JSON field stream should not emit frontend stream delta on node %q; callCount=%d",
		summaryNodeID, callCount)
}

func TestReActLoop_AITagChineseStreamKeepsUTF8(t *testing.T) {
	const (
		factsNodeID = "test-chinese-aitag-node"
		factsTag    = "FACTS"
		factsField  = "facts"
		expected    = "中文事实：用于验证首字节预读不会打坏 UTF-8。"
	)

	var (
		eventsMu sync.Mutex
		events   []*schema.AiOutputEvent
	)

	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			if callCount == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"capture_facts"}<|FACTS_CURRENT_NONCE|>` + expected + `<|FACTS_END_CURRENT_NONCE|>`,
				))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish","answer":"done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("chinese-aitag-loop", reactIns,
		reactloops.WithAITagFieldWithAINodeId(factsTag, factsField, factsNodeID, aicommon.TypeTextMarkdown),
		reactloops.WithRegisterLoopAction(
			"capture_facts",
			"capture chinese facts for utf8 stream test",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("chinese-aitag-task", ctx, "test chinese aitag stream utf8")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()
	require.Equal(t, expected, collectNodeStreamContent(events, factsNodeID))
}

func TestReActLoop_FieldChineseStreamKeepsUTF8(t *testing.T) {
	const (
		summaryNodeID = "test-chinese-summary-node"
		expected      = "中文总结：用于验证 JSON field 首字节预读不会乱码。"
	)

	var (
		eventsMu sync.Mutex
		events   []*schema.AiOutputEvent
	)

	callCount := 0
	reactIns, err := aireact.NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			callCount++
			rsp := i.NewAIResponse()
			if callCount == 1 {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"capture_summary","summary":"` + expected + `"}`))
			} else {
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish","answer":"done"}`))
			}
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			eventsMu.Lock()
			defer eventsMu.Unlock()
			events = append(events, e)
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop("chinese-field-loop", reactIns,
		reactloops.WithRegisterLoopActionWithStreamField(
			"capture_summary",
			"capture chinese summary for utf8 stream test",
			nil,
			[]*reactloops.LoopStreamField{{
				FieldName:   "summary",
				AINodeId:    summaryNodeID,
				ContentType: aicommon.TypeTextMarkdown,
			}},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("chinese-field-task", ctx, "test chinese field stream utf8")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()
	require.Equal(t, expected, collectNodeStreamContent(events, summaryNodeID))
}
