package aireact

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// 关键词: enforceTodoCompletionBeforeSatisfaction 单测, Satisfied 兜底回退,
//
//	[VERIFICATION_TODO_INCOMPLETE], 端到端 VerifyUserSatisfaction 覆盖

// helper: 把当前 ReAct 实例的 timeline 渲染成 string 供子串断言用. 这里用
// ToTimelineItemOutputLastN 直接读结构化输出, 避免依赖具体 prompt 模板.
func dumpReactTimeline(t *testing.T, r *ReAct) string {
	t.Helper()
	items := r.config.Timeline.ToTimelineItemOutputLastN(64)
	var buf bytes.Buffer
	for _, item := range items {
		if item == nil {
			continue
		}
		buf.WriteString(item.Type)
		buf.WriteString(" ")
		buf.WriteString(item.Content)
		buf.WriteString("\n")
	}
	return buf.String()
}

func setVerificationTestCurrentTask(r *ReAct, ctx context.Context, taskID string) {
	r.SetCurrentTask(aicommon.NewStatefulTaskBase(taskID, "verification test user input", ctx, r.config.GetEmitter(), true))
}

// TestEnforceTodoCompletionBeforeSatisfaction_OverridesWhenActive 验证兜底
// 机制: 当 AI 声明 user_satisfied=true 但全局 TODO store 仍有 PENDING 项,
// 必须把 Satisfied 强制回退为 false, 同时:
//   - reasoning 注入 [OVERRIDE] 前缀, 并保留 AI 原 reasoning 在 [AI ORIGINAL]
//   - timeline 写入 [VERIFICATION_TODO_INCOMPLETE] 推动下一轮
//
// 关键词: Satisfied 兜底回退, OVERRIDE 注入, VERIFICATION_TODO_INCOMPLETE
func TestEnforceTodoCompletionBeforeSatisfaction_OverridesWhenActive(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "verify_target", Content: "复现错误码"},
		},
	})

	result := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		Reasoning: "目标达成: 已成功复现",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.False(t, result.Satisfied,
		"Satisfied must be force-overridden to false while active TODO(s) remain in the global store")
	require.Contains(t, result.Reasoning, "[OVERRIDE]")
	require.Contains(t, result.Reasoning, "user_satisfied has been force-overridden to false")
	require.Contains(t, result.Reasoning, "verify_target")
	require.Contains(t, result.Reasoning, "[AI ORIGINAL]")
	require.Contains(t, result.Reasoning, "目标达成: 已成功复现",
		"the AI's original reasoning must be preserved verbatim for audit traceability")

	dumped := dumpReactTimeline(t, react)
	require.Contains(t, dumped, "[VERIFICATION_TODO_INCOMPLETE]",
		"timeline must carry a [VERIFICATION_TODO_INCOMPLETE] breadcrumb to push the AI on the next round")
	require.Contains(t, dumped, "verify_target")
}

// TestEnforceTodoCompletionBeforeSatisfaction_OverridesEvenWhenReasoningEmpty
// 验证 reasoning 为空时的兜底路径: 不应该崩, [OVERRIDE] 仍要写入.
//
// 关键词: 空 reasoning 兜底, 边界条件
func TestEnforceTodoCompletionBeforeSatisfaction_OverridesEvenWhenReasoningEmpty(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "doing_one", Content: "推进中"},
			{Op: "doing", ID: "doing_one"},
		},
	})

	result := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		Reasoning: "",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.False(t, result.Satisfied)
	require.True(t, strings.HasPrefix(result.Reasoning, "[OVERRIDE]"),
		"empty AI reasoning should be replaced by an override-only reasoning prefix")
	require.NotContains(t, result.Reasoning, "[AI ORIGINAL]",
		"no [AI ORIGINAL] section should appear when the AI did not provide its own reasoning")
}

// TestEnforceTodoCompletionBeforeSatisfaction_KeepsSatisfiedWhenAllClosed
// 验证全部 TODO 都被显式关闭 (done/delete/skip) 后, Satisfied=true 不会
// 被回退, reasoning 保持原文, timeline 不写入 [VERIFICATION_TODO_INCOMPLETE].
//
// 关键词: 全部关闭后 Satisfied 保持, 无 OVERRIDE, 无 INCOMPLETE timeline
func TestEnforceTodoCompletionBeforeSatisfaction_KeepsSatisfiedWhenAllClosed(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "done_one", Content: "完成项"},
			{Op: "add", ID: "deleted_one", Content: "删除项"},
			{Op: "add", ID: "skipped_one", Content: "跳过项"},
		},
	})
	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "done", ID: "done_one"},
			{Op: "delete", ID: "deleted_one"},
			{Op: "skip", ID: "skipped_one"},
		},
	})

	result := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		Reasoning: "目标达成",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.True(t, result.Satisfied,
		"Satisfied must remain true when every tracked TODO has been explicitly closed via done/delete/skip")
	require.Equal(t, "目标达成", result.Reasoning,
		"reasoning must NOT be touched when no override is required")
	dumped := dumpReactTimeline(t, react)
	require.NotContains(t, dumped, "[VERIFICATION_TODO_INCOMPLETE]",
		"no override timeline breadcrumb should be emitted when all TODOs are closed")
}

// TestEnforceTodoCompletionBeforeSatisfaction_NoStoreNoChange 验证空 store
// + Satisfied=true 时无变化. 这是最常见的"短任务无 TODO 直接完成"路径.
//
// 关键词: 空 store 直通, Satisfied 保持, 短任务路径
func TestEnforceTodoCompletionBeforeSatisfaction_NoStoreNoChange(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	result := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		Reasoning: "无 TODO, 直接完成",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.True(t, result.Satisfied)
	require.Equal(t, "无 TODO, 直接完成", result.Reasoning)
}

// TestEnforceTodoCompletionBeforeSatisfaction_NoChangeWhenAlreadyUnsatisfied
// 验证 Satisfied=false 时 helper 直接返回, 不会写 timeline 也不会改 reasoning.
//
// 关键词: false 短路, 兜底仅在 Satisfied=true 时触发
func TestEnforceTodoCompletionBeforeSatisfaction_NoChangeWhenAlreadyUnsatisfied(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "x", Content: "未完成"},
		},
	})

	result := &aicommon.VerifySatisfactionResult{
		Satisfied: false,
		Reasoning: "继续推进",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.False(t, result.Satisfied)
	require.Equal(t, "继续推进", result.Reasoning)
	dumped := dumpReactTimeline(t, react)
	require.NotContains(t, dumped, "[VERIFICATION_TODO_INCOMPLETE]")
}

// TestVerifyUserSatisfaction_OverridesSatisfiedEndToEnd 端到端验证: mock
// AI 返回 user_satisfied=true 但 store 仍有未关闭 TODO 时, 真实的
// VerifyUserSatisfaction 调用应该返回 Satisfied=false 的 result, 让
// operator 路径 (tool_call_common.go 等) 正确地 Continue 而非 Exit.
//
// 关键词: VerifyUserSatisfaction 端到端兜底, mock AI Satisfied 被推翻,
//
//	operator Continue 路径
func TestVerifyUserSatisfaction_OverridesSatisfiedEndToEnd(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(
				`{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"mock-AI-says-done","completed_task_index":"1-1"}`,
			))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "unfinished_step", Content: "尚未完成的步骤"},
		},
	})

	result, verifyErr := react.VerifyUserSatisfaction(context.Background(), "demo-query", false, "demo-payload")
	require.NoError(t, verifyErr)
	require.NotNil(t, result)

	require.False(t, result.Satisfied,
		"mock AI declared user_satisfied=true but the store has an active TODO; the bottom-line override must kick in")
	require.Contains(t, result.Reasoning, "[OVERRIDE]")
	require.Contains(t, result.Reasoning, "unfinished_step")
	require.Contains(t, result.Reasoning, "[AI ORIGINAL] mock-AI-says-done")

	dumped := dumpReactTimeline(t, react)
	require.Contains(t, dumped, "[VERIFICATION_TODO_INCOMPLETE]")
}

func TestEnforceTodoCompletionBeforeSatisfaction_IgnoresSiblingTaskTodos(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"object"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)

	setVerificationTestCurrentTask(react, context.Background(), "sibling-task")
	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "sibling_only", Content: "兄弟任务的待办"},
		},
	})

	setVerificationTestCurrentTask(react, context.Background(), "current-task")
	result := &aicommon.VerifySatisfactionResult{
		Satisfied: true,
		Reasoning: "当前任务已完成",
	}
	react.enforceTodoCompletionBeforeSatisfaction(result)

	require.True(t, result.Satisfied)
	require.Equal(t, "当前任务已完成", result.Reasoning)
	require.NotContains(t, dumpReactTimeline(t, react), "[VERIFICATION_TODO_INCOMPLETE]")
}

// TestVerifyUserSatisfaction_KeepsSatisfiedWhenAITouchesAllTodos 端到端
// 验证: mock AI 在同一轮内通过 next_movements 显式关闭所有残留 TODO 并
// 声明 user_satisfied=true 时, 结果应保持 Satisfied=true. 这是 prompt 中
// 引导 AI 走的"宣告完成前同轮先清理 TODO"路径.
//
// 关键词: 同轮关闭 TODO + Satisfied=true 直通, prompt 引导路径
func TestVerifyUserSatisfaction_KeepsSatisfiedWhenAITouchesAllTodos(t *testing.T) {
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(
				`{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"all closed, done","completed_task_index":"1-1","next_movements":[{"op":"done","id":"finished_step"},{"op":"skip","id":"obsolete_step"}]}`,
			))
			rsp.Close()
			return rsp, nil
		}),
	)
	require.NoError(t, err)
	setVerificationTestCurrentTask(react, context.Background(), "current-task")

	react.AppendVerificationHistory(&aicommon.VerifySatisfactionResult{
		Satisfied: false,
		NextMovements: []aicommon.VerifyNextMovement{
			{Op: "add", ID: "finished_step", Content: "已完成的步骤"},
			{Op: "add", ID: "obsolete_step", Content: "本次范围内放弃推进的步骤"},
		},
	})

	result, verifyErr := react.VerifyUserSatisfaction(context.Background(), "demo-query", false, "demo-payload")
	require.NoError(t, verifyErr)
	require.NotNil(t, result)
	require.True(t, result.Satisfied,
		"when AI explicitly closes every active TODO in the same round, Satisfied=true must survive")
	require.Equal(t, "all closed, done", result.Reasoning,
		"reasoning must NOT be wrapped with [OVERRIDE] when no override happened")

	dumped := dumpReactTimeline(t, react)
	require.NotContains(t, dumped, "[VERIFICATION_TODO_INCOMPLETE]")
}
