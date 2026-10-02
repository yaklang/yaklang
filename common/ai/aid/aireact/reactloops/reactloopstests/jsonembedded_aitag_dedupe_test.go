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

// JSON 字段内的 AITag 包裹只能输出一次干净的内层内容。
func TestReActLoop_FieldStreamHandler_DropsJSONEmbeddedAITag(t *testing.T) {
	const (
		contentNodeID  = "test-content-stream-node"
		contentTagName = "CONTENT"
		contentField   = "content"
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
				// 第 1 轮: AI 把整段 AITag wrapper 塞进 JSON content 字段值
				// (用 CURRENT_NONCE 占位符, 系统已通过 ExtraNonces 兜底)
				buggy := `{"@action":"capture_content","content":"<|CONTENT_CURRENT_NONCE|>\n## 测试事实\n- inner content line 1\n- inner content line 2\n<|CONTENT_END_CURRENT_NONCE|>"}`
				rsp.EmitOutputStream(bytes.NewBufferString(buggy))
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
	require.NoError(t, err, "failed to create test ReAct instance")

	loop, err := reactloops.NewReActLoop("dedupe-loop", reactIns,
		reactloops.WithAITagFieldWithAINodeId(
			contentTagName, contentField, contentNodeID, aicommon.TypeTextMarkdown,
		),
		reactloops.WithRegisterLoopAction(
			"capture_content",
			"capture content via AITag stream for dedupe test",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err, "failed to construct ReActLoop")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("dedupe-task", ctx, "test json-embedded aitag dedup")
	require.NoError(t, err, "loop execute should not fail")

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	streamStartCount := 0
	var collectedDeltas []string
	for _, e := range events {
		if e == nil {
			continue
		}
		if e.NodeId != contentNodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START {
			streamStartCount++
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			collectedDeltas = append(collectedDeltas, string(e.StreamDelta))
		}
	}

	require.Equalf(t, 1, streamStartCount,
		"expected exactly 1 STREAM_START on node %q (the JSON-embedded AITag wrapper "+
			"duplicate must be dropped, only AITag stream path should emit clean inner content); "+
			"got %d. callCount=%d. deltas=%q",
		contentNodeID, streamStartCount, callCount, collectedDeltas)

	// 留下的那一份必须是 AITag 路径推出来的干净内层, 不能再带 `<|CONTENT_*|>` wrappers
	combined := ""
	for _, d := range collectedDeltas {
		combined += d
	}
	require.NotContainsf(t, combined, "<|CONTENT_",
		"surviving emit must NOT contain AITag wrapper literal; got combined delta: %q", combined)
	require.NotContainsf(t, combined, "<|"+contentTagName+"_END_",
		"surviving emit must NOT contain AITag end-wrapper literal; got combined delta: %q", combined)
}

// TestReActLoop_FieldStreamHandler_KeepsCleanJSONFieldValue 验证常规 case (JSON
// `content` 字段是干净的 markdown, 没有 AITag wrappers) 不被误判为重复, 仍正常 emit.
func TestReActLoop_FieldStreamHandler_KeepsCleanJSONFieldValue(t *testing.T) {
	const (
		contentNodeID  = "test-content-clean-node"
		contentTagName = "CONTENT"
		contentField   = "content"
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
				// 干净 JSON `content` 字段值, 不含 AITag wrappers
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"capture_content","content":"## 测试事实\n- 干净 markdown 内容\n- 第二行"}`,
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
	require.NoError(t, err, "failed to create test ReAct instance")

	loop, err := reactloops.NewReActLoop("clean-loop", reactIns,
		reactloops.WithAITagFieldWithAINodeId(
			contentTagName, contentField, contentNodeID, aicommon.TypeTextMarkdown,
		),
		reactloops.WithRegisterLoopAction(
			"capture_content",
			"capture content via JSON field stream for clean-path test",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				op.Continue()
			},
		),
		reactloops.WithMaxIterations(3),
	)
	require.NoError(t, err, "failed to construct ReActLoop")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = loop.Execute("clean-task", ctx, "test clean json field path")
	require.NoError(t, err, "loop execute should not fail")

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	streamStartCount := 0
	for _, e := range events {
		if e == nil {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START && e.NodeId == contentNodeID {
			streamStartCount++
		}
	}

	require.GreaterOrEqualf(t, streamStartCount, 1,
		"clean JSON content field value should still produce at least 1 STREAM_START on node %q; "+
			"got %d. The dedupe peek must NOT mistakenly drop clean markdown content",
		contentNodeID, streamStartCount)
	require.LessOrEqualf(t, streamStartCount, 1,
		"clean JSON content field value should produce at most 1 STREAM_START on node %q; "+
			"got %d. There should be no extra emit beyond the JSON field stream",
		contentNodeID, streamStartCount)
}
