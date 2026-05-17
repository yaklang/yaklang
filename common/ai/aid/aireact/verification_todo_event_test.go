package aireact

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// TestEmitTodoListUpdate_AfterAppendVerificationHistory 验证 emitTodoListUpdate
// 在 AppendVerificationHistory 写入 SessionPromptState 后:
//  1. 发出一条 EVENT_TYPE_TODO_LIST_UPDATE 事件 (NodeId = "todo_list")
//  2. payload.items 与 stats 来自最新的 SessionPromptState 快照
//  3. payload.applied_ops 是这一轮的增量 movements
//  4. payload.satisfied 与 VerifySatisfactionResult.Satisfied 一致
//
// 这是计划中 "emit_in_verify" todo 的核心验收点。
//
// 关键词: EVENT_TYPE_TODO_LIST_UPDATE 发射断言, AppendVerificationHistory 后 emit,
//
//	全局 TODO 通道
func TestEmitTodoListUpdate_AfterAppendVerificationHistory(t *testing.T) {
	var (
		mu       sync.Mutex
		captured []*schema.AiOutputEvent
	)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.Type != schema.EVENT_TYPE_TODO_LIST_UPDATE {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, e)
		}),
	)
	require.NoError(t, err)

	// First round: introduce two TODOs
	first := &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "verify_target", Content: "复现错误码"},
			{Op: "add", ID: "collect_signal", Content: "采集响应特征"},
		},
	}
	react.AppendVerificationHistory(first)
	react.emitTodoListUpdate(first)

	// Second round: close one TODO and add another
	second := &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "done", ID: "verify_target"},
			{Op: "add", ID: "replay_payload", Content: "更换 payload 复测"},
		},
	}
	react.AppendVerificationHistory(second)
	react.emitTodoListUpdate(second)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, captured, 2, "expected 2 todo_list_update events, got %d", len(captured))

	for _, evt := range captured {
		require.Equal(t, schema.EVENT_TYPE_TODO_LIST_UPDATE, evt.Type)
		require.Equal(t, "todo_list", evt.NodeId)
		require.True(t, evt.IsJson)
	}

	var firstPayload aicommon.TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured[0].Content, &firstPayload))
	require.Len(t, firstPayload.Items, 2)
	require.Equal(t, 2, firstPayload.Stats.Pending)
	require.Equal(t, 2, len(firstPayload.AppliedOps))
	require.False(t, firstPayload.Satisfied)

	var secondPayload aicommon.TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured[1].Content, &secondPayload))
	require.Len(t, secondPayload.Items, 3)
	require.Equal(t, 1, secondPayload.Stats.Done)
	require.Equal(t, 2, secondPayload.Stats.Pending)
	require.Equal(t, 2, len(secondPayload.AppliedOps))
	doneOpFound := false
	addOpFound := false
	for _, op := range secondPayload.AppliedOps {
		if op.Op == "done" && op.ID == "verify_target" {
			doneOpFound = true
		}
		if op.Op == "add" && op.ID == "replay_payload" {
			addOpFound = true
		}
	}
	require.True(t, doneOpFound, "second round applied_ops must surface the done op")
	require.True(t, addOpFound, "second round applied_ops must surface the new add op")
}

// TestEmitTodoListUpdate_SatisfiedFlipsActiveItemsToSkipped 验证当
// VerifySatisfactionResult.Satisfied == true 时, 事件 payload 中:
//  1. 剩余 PENDING / DOING 都已经被翻成 SKIPPED (与 prompt 渲染保持一致);
//  2. stats.skipped 反映新增的 skipped 数;
//  3. applied_ops 即使为空, 也必须是 [] 而非 null (前端契约)。
//
// 关键词: satisfied SKIPPED 转换, 事件 payload, 空 applied_ops 归一化
func TestEmitTodoListUpdate_SatisfiedFlipsActiveItemsToSkipped(t *testing.T) {
	var (
		mu       sync.Mutex
		captured []*schema.AiOutputEvent
	)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.Type != schema.EVENT_TYPE_TODO_LIST_UPDATE {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, e)
		}),
	)
	require.NoError(t, err)

	first := &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "collect_signal", Content: "采集响应特征"},
			{Op: "add", ID: "replay_payload", Content: "复测 payload"},
		},
	}
	react.AppendVerificationHistory(first)
	react.emitTodoListUpdate(first)

	satisfied := &aicommon.VerifySatisfactionResult{
		Satisfied:     true,
		NextMovements: nil,
	}
	react.AppendVerificationHistory(satisfied)
	react.emitTodoListUpdate(satisfied)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, captured, 2)

	var satisfiedPayload aicommon.TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured[1].Content, &satisfiedPayload))
	require.True(t, satisfiedPayload.Satisfied)
	require.Equal(t, 2, satisfiedPayload.Stats.Skipped)
	require.Zero(t, satisfiedPayload.Stats.Pending)
	require.Zero(t, satisfiedPayload.Stats.Doing)
	require.NotNil(t, satisfiedPayload.AppliedOps, "AppliedOps must be [] not null")
	require.Empty(t, satisfiedPayload.AppliedOps)
}
