package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 关键词: VerificationTodoStore 单测, TODO 增量, satisfied SKIPPED 转换,
//
//	persisted JSON 解析

func TestVerificationTodoStore_ApplyAndRender(t *testing.T) {
	store := NewVerificationTodoStore()
	require.True(t, store.IsEmpty())

	store.Apply(false, []VerifyNextMovement{
		{Op: "add", ID: "collect_signal", Content: "收集页面响应信号"},
		{Op: "add", ID: "fix_title", Content: "修正标题"},
	})
	store.Apply(false, []VerifyNextMovement{
		{Op: "done", ID: "collect_signal"},
		{Op: "delete", ID: "fix_title"},
		{Op: "add", ID: "replay_payload", Content: "使用新 payload 复测"},
	})

	rendered := store.Render()
	require.Contains(t, rendered, "- [x]: [id: collect_signal]: 收集页面响应信号")
	require.Contains(t, rendered, "- [DELETED]: [id: fix_title]: 修正标题")
	require.Contains(t, rendered, "- [ ]: [id: replay_payload]: 使用新 payload 复测")
}

func TestVerificationTodoStore_ApplySatisfiedFlipsActiveToSkipped(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(false, []VerifyNextMovement{
		{Op: "add", ID: "collect_signal", Content: "收集页面回显信号"},
		{Op: "add", ID: "retry_payload", Content: "更换 payload 再次验证"},
	})
	store.Apply(true, nil)

	stats := store.Stats()
	require.Equal(t, 2, stats.Skipped)
	require.Zero(t, stats.Pending)
	require.Zero(t, stats.Doing)

	rendered := store.Render()
	require.Contains(t, rendered, "- [SKIPPED]: [id: collect_signal]: 收集页面回显信号")
	require.Contains(t, rendered, "- [SKIPPED]: [id: retry_payload]: 更换 payload 再次验证")
}

func TestVerificationTodoStore_DoingPreservesContentUpdate(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(false, []VerifyNextMovement{{Op: "add", ID: "rename_file", Content: "先创建临时文件"}})
	store.Apply(false, []VerifyNextMovement{
		{Op: "add", ID: "rename_file", Content: "重命名临时文件为最终名称"},
		{Op: "doing", ID: "rename_file"},
	})

	items := store.SnapshotItems()
	require.Len(t, items, 1)
	require.Equal(t, VerificationTodoStatusDoing, items[0].Status)
	require.Equal(t, "重命名临时文件为最终名称", items[0].Content)
}

func TestVerificationTodoStore_MarshalRoundtrip(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(false, []VerifyNextMovement{{Op: "add", ID: "rename_file", Content: "重命名临时文件"}})
	encoded := store.Marshal()
	require.Contains(t, encoded, `"id":"rename_file"`)
	require.Contains(t, encoded, `"status":"PENDING"`)

	restored := UnmarshalVerificationTodoStore(encoded)
	require.False(t, restored.IsEmpty())
	require.Equal(t, 1, restored.Stats().Pending)

	emptyRestored := UnmarshalVerificationTodoStore("")
	require.True(t, emptyRestored.IsEmpty())

	malformedRestored := UnmarshalVerificationTodoStore("not-json")
	require.True(t, malformedRestored.IsEmpty())
}

func TestVerificationTodoStore_RenderMarkdownDeltaIsReadOnly(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(false, []VerifyNextMovement{{Op: "add", ID: "old_pending", Content: "旧任务"}})

	delta := store.RenderMarkdownDelta(false, []VerifyNextMovement{
		{Op: "done", ID: "old_pending"},
		{Op: "add", ID: "new_task", Content: "新增任务"},
	})
	require.Contains(t, delta, "(new)")
	require.Contains(t, delta, "(done)")
	require.Contains(t, delta, "~~旧任务~~")

	// preview must not have mutated underlying state
	stats := store.Stats()
	require.Equal(t, 1, stats.Pending)
	require.Zero(t, stats.Done)
}

func TestSessionPromptState_ApplyVerificationTodoOps_AccumulatesAndRenders(t *testing.T) {
	state := NewSessionPromptState()
	state.ApplyVerificationTodoOps(false, []VerifyNextMovement{
		{Op: "add", ID: "step1", Content: "第一步"},
	})
	state.ApplyVerificationTodoOps(false, []VerifyNextMovement{
		{Op: "add", ID: "step2", Content: "第二步"},
		{Op: "doing", ID: "step1"},
	})

	rendered := state.GetVerificationTodoRendered()
	require.Contains(t, rendered, "- [DOING]: [id: step1]: 第一步")
	require.Contains(t, rendered, "- [ ]: [id: step2]: 第二步")

	stats := state.GetVerificationTodoStats()
	require.Equal(t, 1, stats.Doing)
	require.Equal(t, 1, stats.Pending)

	items := state.SnapshotVerificationTodoItems()
	require.Len(t, items, 2)
}

func TestSessionPromptState_GetVerificationTodoRendered_EmptyReturnsEmpty(t *testing.T) {
	state := NewSessionPromptState()
	require.Equal(t, "", state.GetVerificationTodoRendered(),
		"empty state should render empty string so the prompt template can skip the block")
}

func TestSessionPromptState_GetVerificationTodoMarkdownDelta_IsPreview(t *testing.T) {
	state := NewSessionPromptState()
	state.ApplyVerificationTodoOps(false, []VerifyNextMovement{
		{Op: "add", ID: "step1", Content: "第一步"},
	})

	delta := state.GetVerificationTodoMarkdownDelta(false, []VerifyNextMovement{
		{Op: "done", ID: "step1"},
	})
	require.Contains(t, delta, "(done)")

	// preview must not have mutated underlying state
	stats := state.GetVerificationTodoStats()
	require.Equal(t, 1, stats.Pending)
	require.Zero(t, stats.Done)
}
