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

// TestEmitTodoListUpdate_ExplicitSkipShowsSkipped 验证显式 skip op 在
// EVENT_TYPE_TODO_LIST_UPDATE payload 中的呈现:
//  1. 显式 skip 后 stats.Skipped 增加, Pending 减少;
//  2. applied_ops 携带 skip op 增量;
//  3. AI 同一轮内关闭所有活跃 TODO 后, Satisfied=true 在 payload 中保留.
//
// 此前的旧测试 (TestEmitTodoListUpdate_SatisfiedFlipsActiveItemsToSkipped)
// 依赖"Satisfied=true 自动翻 SKIPPED"语义, 该语义已被废弃, 替换为本测试.
//
// 关键词: 显式 skip 事件 payload, applied_ops 增量, Satisfied 同轮关闭后保留
func TestEmitTodoListUpdate_ExplicitSkipShowsSkipped(t *testing.T) {
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

	// 同一轮内: AI 显式关闭两条残留 TODO (一条 done 一条 skip), 再宣告 Satisfied=true.
	// 经过 enforceTodoCompletionBeforeSatisfaction (在真实 VerifyUserSatisfaction
	// 链路中) 后 Satisfied 才能稳定保留 true; 这里直接调 AppendVerificationHistory
	// 模拟 store 已被关闭的状态, 然后 emit 检查 payload.
	satisfied := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "done", ID: "collect_signal"},
			{Op: "skip", ID: "replay_payload"},
		},
	}
	react.AppendVerificationHistory(satisfied)
	react.emitTodoListUpdate(satisfied)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, captured, 2)

	var satisfiedPayload aicommon.TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured[1].Content, &satisfiedPayload))
	require.True(t, satisfiedPayload.Satisfied)
	require.Equal(t, 1, satisfiedPayload.Stats.Done,
		"the done op should yield exactly one DONE item")
	require.Equal(t, 1, satisfiedPayload.Stats.Skipped,
		"the explicit skip op should yield exactly one SKIPPED item")
	require.Zero(t, satisfiedPayload.Stats.Pending)
	require.Zero(t, satisfiedPayload.Stats.Doing)
	require.NotNil(t, satisfiedPayload.AppliedOps, "AppliedOps must be [] not null")
	require.Len(t, satisfiedPayload.AppliedOps, 2)

	doneSeen := false
	skipSeen := false
	for _, op := range satisfiedPayload.AppliedOps {
		if op.Op == "done" && op.ID == "collect_signal" {
			doneSeen = true
		}
		if op.Op == "skip" && op.ID == "replay_payload" {
			skipSeen = true
		}
	}
	require.True(t, doneSeen, "applied_ops must carry the done op")
	require.True(t, skipSeen, "applied_ops must carry the explicit skip op")
}

// TestEmitTodoListUpdate_SatisfiedRawDoesNotAutoSkip 验证当调用方仅经
// AppendVerificationHistory 路径 (不经过 enforceTodoCompletionBeforeSatisfaction)
// 提交 Satisfied=true 与残留 PENDING 时, store 不再自动翻 SKIPPED. 这是
// 兜底机制成立的底层不变式: store 必须忠实反映 AI 操作, 不能"擅自帮忙关闭".
//
// 关键词: AppendVerificationHistory 不再自动翻, store 客观反馈不变式
func TestEmitTodoListUpdate_SatisfiedRawDoesNotAutoSkip(t *testing.T) {
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

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "left_over", Content: "未处理"},
		},
	})
	rawSatisfied := &aicommon.VerifySatisfactionResult{
		Satisfied:     true,
		NextMovements: nil,
	}
	react.AppendVerificationHistory(rawSatisfied)
	react.emitTodoListUpdate(rawSatisfied)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, captured, 1)
	var payload aicommon.TodoListUpdatePayload
	require.NoError(t, json.Unmarshal(captured[0].Content, &payload))
	require.True(t, payload.Satisfied, "this path bypasses enforce; satisfied is whatever caller supplied")
	require.Equal(t, 1, payload.Stats.Pending,
		"the active TODO must remain PENDING after the satisfied=true history entry; auto-flip semantics are gone")
	require.Zero(t, payload.Stats.Skipped)
}
