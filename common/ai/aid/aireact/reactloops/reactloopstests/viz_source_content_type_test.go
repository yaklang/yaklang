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

// TestReActLoop_FieldStream_EmptyContentTypeFallsToDefault 验证 JSON 字段流路径
// 在 LoopStreamField.ContentType 为空时, 落到 emitter 的 "default" 兜底, 而不是被
// 错误地标成 "text/plain".
//
// 背景: commit 1de91a147 (viz 流来源迁移到独立 VizSource 字段) 在 callAITransaction
// 的 JSON 字段流 handler 里顺手加了 `if contentType == "" { contentType = "text/plain" }`,
// 但 emitter.emitStreamEvent 已有 `if e.contentType == "" { e.contentType = "default" }`
// 兜底. 加 text/plain 会让那些故意不设 ContentType 的字段 (如 http_fuzztest 的 reason
// 字段, 只设 FieldName + AINodeId) 被错误标成 text/plain, 破坏前端按 MIME 主类型解析.
// 本测试锁定该回归: 空 ContentType 必须落到 "default".
//
// 关键词: ContentType 空兜底, text/plain 回归, default content type, VizSource 迁移
func TestReActLoop_FieldStream_EmptyContentTypeFallsToDefault(t *testing.T) {
	const (
		reasonNodeID = "test-empty-ct-reason-node"
		reasonField  = "reason"
		reasonBody   = "因为 GET 方法最常见，先测试它。"
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
					`{"@action":"capture_reason","reason":"` + reasonBody + `"}`,
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

	// 故意不设 ContentType, 模拟 loop_http_fuzztest 的 reason 字段
	// (只设 FieldName + AINodeId).
	loop, err := reactloops.NewReActLoop("empty-ct-loop", reactIns,
		reactloops.WithRegisterLoopActionWithStreamField(
			"capture_reason",
			"capture reason with empty content type",
			nil,
			[]*reactloops.LoopStreamField{{
				FieldName:  reasonField,
				AINodeId:   reasonNodeID,
				// ContentType 留空
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

	err = loop.Execute("empty-ct-task", ctx, "test empty content type field stream")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var streamStart *schema.AiOutputEvent
	var deltaContent string
	for _, e := range events {
		if e == nil || e.NodeId != reasonNodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START && streamStart == nil {
			streamStart = e
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			deltaContent += string(e.StreamDelta)
		}
	}

	require.NotNil(t, streamStart, "expected a STREAM_START event on node %q", reasonNodeID)
	// 核心: 空 ContentType 必须落到 "default", 而不是被错误标成 "text/plain".
	require.Equal(t, "default", streamStart.ContentType,
		"empty ContentType must fall back to \"default\" (emitter default), not \"text/plain\"")
	// VizSource 必须等于字段名, 验证 viz 流来源迁移仍生效.
	require.Equal(t, reasonField, streamStart.VizSource,
		"VizSource must carry the field name so viz can distinguish stream origin")
	require.Equal(t, reasonBody, deltaContent)
}

// TestReActLoop_FieldStream_ExplicitContentTypePreserved 验证显式声明的 ContentType
// (如 text/markdown) 被原样透传到 STREAM_START 事件, 不被空兜底逻辑覆盖.
//
// 关键词: ContentType 透传, text/markdown 保留, VizSource 字段名
func TestReActLoop_FieldStream_ExplicitContentTypePreserved(t *testing.T) {
	const (
		answerNodeID = "test-explicit-ct-conclusion-node"
		answerField  = "conclusion"
		answerBody   = "# 结论\n\n这是一份 **markdown** 回答。"
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
					`{"@action":"capture_conclusion","conclusion":"` + answerBody + `"}`,
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

	loop, err := reactloops.NewReActLoop("explicit-ct-loop", reactIns,
		reactloops.WithRegisterLoopActionWithStreamField(
			"capture_conclusion",
			"capture answer with explicit content type",
			nil,
			[]*reactloops.LoopStreamField{{
				FieldName:   answerField,
				AINodeId:    answerNodeID,
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

	err = loop.Execute("explicit-ct-task", ctx, "test explicit content type field stream")
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var streamStart *schema.AiOutputEvent
	var deltaContent string
	for _, e := range events {
		if e == nil || e.NodeId != answerNodeID {
			continue
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START && streamStart == nil {
			streamStart = e
		}
		if e.Type == schema.EVENT_TYPE_STREAM && e.IsStream && len(e.StreamDelta) > 0 {
			deltaContent += string(e.StreamDelta)
		}
	}

	require.NotNil(t, streamStart, "expected a STREAM_START event on node %q", answerNodeID)
	require.Equal(t, aicommon.TypeTextMarkdown, streamStart.ContentType,
		"explicit ContentType (text/markdown) must be preserved on STREAM_START")
	require.Equal(t, answerField, streamStart.VizSource,
		"VizSource must carry the field name even when ContentType is explicit")
	require.Equal(t, answerBody, deltaContent)
}
