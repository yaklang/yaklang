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

// TestReActLoop_FieldStreamHandler_DropsJSONEmbeddedAITag 验证当 AI 把 AITag 包裹
// (例如 <|FACTS_xxx|>...<|FACTS_END_xxx|>) 整段塞进 JSON 字符串字段值时, 字段流
// handler 不会重复 emit 一份带 wrappers 的 raw 文本.
//
// 触发场景: AI 误将示例 prompt 里的 AITag 块写进了 JSON `facts` 字段值,
//
//	ActionMaker 会同时通过 JSON 字段流路径与 AITag 流路径触发同一个 field stream
//	handler, 各 emit 一次, 前端就会看到两条 "事实" 事件 — 一条是裹着 `<|FACTS_*|>`
//	的丑文本, 另一条是干净的 markdown.
//
// 期望行为: peek 检测到 JSON 路径流以 `<|TagName_` 开头时, 静默 drain 不 emit;
//
//	AITag 路径仍正常 emit 干净的内层. 因此 STREAM_START 在该 nodeId 上只出现一次.
//
// 关键词: JSON-embedded AITag dedup, FACTS 重复事件修复, peek 检测 wrappers 前缀
func TestReActLoop_FieldStreamHandler_DropsJSONEmbeddedAITag(t *testing.T) {
	const (
		factsNodeID  = "test-facts-stream-node"
		factsTagName = "FACTS"
		factsField   = "facts"
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
				// 第 1 轮: AI 把整段 AITag wrapper 塞进 JSON facts 字段值
				// (用 CURRENT_NONCE 占位符, 系统已通过 ExtraNonces 兜底)
				buggy := `{"@action":"capture_facts","facts":"<|FACTS_CURRENT_NONCE|>\n## 测试事实\n- inner content line 1\n- inner content line 2\n<|FACTS_END_CURRENT_NONCE|>"}`
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
			factsTagName, factsField, factsNodeID, aicommon.TypeTextMarkdown,
		),
		reactloops.WithRegisterLoopAction(
			"capture_facts",
			"capture facts via AITag stream for dedupe test",
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
		if e.NodeId != factsNodeID {
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
		factsNodeID, streamStartCount, callCount, collectedDeltas)

	// 留下的那一份必须是 AITag 路径推出来的干净内层, 不能再带 `<|FACTS_*|>` wrappers
	combined := ""
	for _, d := range collectedDeltas {
		combined += d
	}
	require.NotContainsf(t, combined, "<|FACTS_",
		"surviving emit must NOT contain AITag wrapper literal; got combined delta: %q", combined)
	require.NotContainsf(t, combined, "<|"+factsTagName+"_END_",
		"surviving emit must NOT contain AITag end-wrapper literal; got combined delta: %q", combined)
}

// TestReActLoop_FieldStreamHandler_KeepsCleanJSONFieldValue 验证常规 case (JSON
// `facts` 字段是干净的 markdown, 没有 AITag wrappers) 不被误判为重复, 仍正常 emit.
func TestReActLoop_FieldStreamHandler_KeepsCleanJSONFieldValue(t *testing.T) {
	const (
		factsNodeID  = "test-facts-clean-node"
		factsTagName = "FACTS"
		factsField   = "facts"
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
				// 干净 JSON `facts` 字段值, 不含 AITag wrappers
				rsp.EmitOutputStream(bytes.NewBufferString(
					`{"@action":"capture_facts","facts":"## 测试事实\n- 干净 markdown 内容\n- 第二行"}`,
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
			factsTagName, factsField, factsNodeID, aicommon.TypeTextMarkdown,
		),
		reactloops.WithRegisterLoopAction(
			"capture_facts",
			"capture facts via JSON field stream for clean-path test",
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
		if e.Type == schema.EVENT_TYPE_STREAM_START && e.NodeId == factsNodeID {
			streamStartCount++
		}
	}

	require.GreaterOrEqualf(t, streamStartCount, 1,
		"clean JSON facts field value should still produce at least 1 STREAM_START on node %q; "+
			"got %d. The dedupe peek must NOT mistakenly drop clean markdown content",
		factsNodeID, streamStartCount)
	require.LessOrEqualf(t, streamStartCount, 1,
		"clean JSON facts field value should produce at most 1 STREAM_START on node %q; "+
			"got %d. There should be no extra emit beyond the JSON field stream",
		factsNodeID, streamStartCount)
}
