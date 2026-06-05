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

	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "collect_signal", Content: "收集页面响应信号"},
		{Op: "add", ID: "fix_title", Content: "修正标题"},
	})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "done", ID: "collect_signal"},
		{Op: "delete", ID: "fix_title"},
		{Op: "add", ID: "replay_payload", Content: "使用新 payload 复测"},
	})

	rendered := store.Render()
	require.Contains(t, rendered, "- [x]: [id: collect_signal]: 收集页面响应信号")
	require.Contains(t, rendered, "- [DELETED]: [id: fix_title]: 修正标题")
	require.Contains(t, rendered, "- [ ]: [id: replay_payload]: 使用新 payload 复测")
}

// TestVerificationTodoStore_ApplySatisfiedDoesNotAutoSkip 验证旧"satisfied
// 自动翻 SKIPPED"语义已被废弃: 当 Apply(true, nil) 被调用时, store 不应该
// 自行把残留 PENDING/DOING 改成 SKIPPED. AI 必须通过显式 done / delete /
// skip op 才能关闭 TODO.
//
// 关键词: 取消自动翻转, 显式关闭, Satisfied 兜底前置条件
func TestVerificationTodoStore_ApplySatisfiedDoesNotAutoSkip(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "collect_signal", Content: "收集页面回显信号"},
		{Op: "add", ID: "retry_payload", Content: "更换 payload 再次验证"},
	})
	store.Apply(VerificationTodoScope{}, true, nil)

	stats := store.Stats()
	require.Zero(t, stats.Skipped, "satisfied flag must NOT auto-flip active TODOs to SKIPPED anymore")
	require.Equal(t, 2, stats.Pending, "pending TODOs must stay PENDING until AI explicitly closes them")
	require.Zero(t, stats.Doing)

	rendered := store.Render()
	require.Contains(t, rendered, "- [ ]: [id: collect_signal]: 收集页面回显信号")
	require.Contains(t, rendered, "- [ ]: [id: retry_payload]: 更换 payload 再次验证")
	require.NotContains(t, rendered, "[SKIPPED]")
}

// TestVerificationTodoStore_ExplicitSkipOpMarksSkipped 验证新增的显式
// `skip` op: AI 主动声明跳过某个 TODO 时, status 切到 SKIPPED, 但 content
// 仍然保留, 让 prompt 渲染能告诉模型"这个 TODO 是被主动跳过的, 不是被
// 删除的".
//
// 关键词: 显式 skip op, SKIPPED 状态, content 保留
func TestVerificationTodoStore_ExplicitSkipOpMarksSkipped(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "stale_idea", Content: "本次范围内不打算继续推进"},
	})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "skip", ID: "stale_idea"},
	})

	stats := store.Stats()
	require.Equal(t, 1, stats.Skipped)
	require.Zero(t, stats.Pending)
	require.Zero(t, stats.Doing)

	items := store.SnapshotItems()
	require.Len(t, items, 1)
	require.Equal(t, VerificationTodoStatusSkipped, items[0].Status)
	require.Equal(t, "本次范围内不打算继续推进", items[0].Content,
		"explicit skip must keep the original content so the prompt can still describe what was skipped")

	rendered := store.Render()
	require.Contains(t, rendered, "- [SKIPPED]: [id: stale_idea]: 本次范围内不打算继续推进")
}

// TestVerificationTodoStore_ExplicitSkipOpCanUpdateContent 验证 skip op 与
// delete op 类似, 当 movement.Content 非空时允许在跳过时一并更新 content
// (例如"跳过原因"). 这给 AI 留下一种语义化的 audit trail 能力.
//
// 关键词: skip op content 覆盖, 跳过原因
func TestVerificationTodoStore_ExplicitSkipOpCanUpdateContent(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "verify_target", Content: "复现错误码"},
	})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "skip", ID: "verify_target", Content: "目标接口已下线，跳过复现"},
	})

	items := store.SnapshotItems()
	require.Len(t, items, 1)
	require.Equal(t, VerificationTodoStatusSkipped, items[0].Status)
	require.Equal(t, "目标接口已下线，跳过复现", items[0].Content)
}

// TestVerificationTodoStore_HasActiveTodos 覆盖 HasActiveTodos 的全部分支:
// 空 store / 仅 pending / 仅 doing / 仅 done / 仅 deleted / 仅 skipped /
// 混合 active 与 closed. 这是 Satisfied 兜底机制最直接依赖的信号源.
//
// 关键词: HasActiveTodos 全分支覆盖, Satisfied 兜底信号
func TestVerificationTodoStore_HasActiveTodos(t *testing.T) {
	t.Run("empty store has no active todos", func(t *testing.T) {
		store := NewVerificationTodoStore()
		require.False(t, store.HasActiveTodos())
	})

	t.Run("pending only is active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "p1", Content: "p"},
		})
		require.True(t, store.HasActiveTodos())
	})

	t.Run("doing only is active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "d1", Content: "d"},
			{Op: "doing", ID: "d1"},
		})
		require.True(t, store.HasActiveTodos())
	})

	t.Run("all closed via done is not active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "x", Content: "x"},
			{Op: "done", ID: "x"},
		})
		require.False(t, store.HasActiveTodos())
	})

	t.Run("all closed via delete is not active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "x", Content: "x"},
			{Op: "delete", ID: "x"},
		})
		require.False(t, store.HasActiveTodos())
	})

	t.Run("all closed via skip is not active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "x", Content: "x"},
			{Op: "skip", ID: "x"},
		})
		require.False(t, store.HasActiveTodos())
	})

	t.Run("mixed active and closed is still active", func(t *testing.T) {
		store := NewVerificationTodoStore()
		store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
			{Op: "add", ID: "a", Content: "a"},
			{Op: "add", ID: "b", Content: "b"},
			{Op: "done", ID: "a"},
		})
		require.True(t, store.HasActiveTodos())
	})

	t.Run("nil store returns false", func(t *testing.T) {
		var store *VerificationTodoStore
		require.False(t, store.HasActiveTodos())
	})
}

// TestVerificationTodoStore_ActiveTodoItems 验证 ActiveTodoItems 仅返回
// PENDING / DOING 项的深拷贝, 并保留原始顺序 (Apply 追加顺序). 这是
// Satisfied 兜底机制用来构造"剩余 TODO 报告"的输入.
//
// 关键词: ActiveTodoItems 顺序保留, 深拷贝, 仅 active 子集
func TestVerificationTodoStore_ActiveTodoItems(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "pending_one", Content: "p1"},
		{Op: "add", ID: "doing_one", Content: "d1"},
		{Op: "add", ID: "done_one", Content: "x"},
		{Op: "add", ID: "skipped_one", Content: "s"},
	})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "doing", ID: "doing_one"},
		{Op: "done", ID: "done_one"},
		{Op: "skip", ID: "skipped_one"},
	})

	active := store.ActiveTodoItems()
	require.Len(t, active, 2)
	require.Equal(t, "pending_one", active[0].ID)
	require.Equal(t, VerificationTodoStatusPending, active[0].Status)
	require.Equal(t, "doing_one", active[1].ID)
	require.Equal(t, VerificationTodoStatusDoing, active[1].Status)

	// 深拷贝: 修改返回切片中的字段不应影响 store 内部状态
	active[0].Content = "mutated"
	require.Equal(t, "p1", store.SnapshotItems()[0].Content,
		"ActiveTodoItems must return a deep copy so callers cannot accidentally mutate the live store")
}

func TestVerificationTodoStore_DoingPreservesContentUpdate(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "add", ID: "rename_file", Content: "先创建临时文件"}})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
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
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "add", ID: "rename_file", Content: "重命名临时文件"}})
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
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{{Op: "add", ID: "old_pending", Content: "旧任务"}})

	delta := store.RenderMarkdownDelta(VerificationTodoScope{}, false, []VerifyNextMovement{
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

// TestVerificationTodoStore_RenderMarkdownDeltaHighlightsCurrentSkip 验证
// 本轮显式 skip op 在 RenderMarkdownDelta 中被打上 (skipped) marker, 而
// 此前轮次已经被 skip 的 TODO 不再带 marker. 这是前端"本轮变化高亮"必须
// 区分新旧 skip 的关键.
//
// 关键词: RenderMarkdownDelta skip 当轮高亮, 新旧 SKIPPED 区分
func TestVerificationTodoStore_RenderMarkdownDeltaHighlightsCurrentSkip(t *testing.T) {
	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "old_skipped", Content: "上一轮就被跳过的任务"},
		{Op: "add", ID: "to_skip_now", Content: "本轮将被跳过的任务"},
	})
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "skip", ID: "old_skipped"},
	})

	delta := store.RenderMarkdownDelta(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "skip", ID: "to_skip_now"},
	})
	require.Contains(t, delta, "(skipped) 本轮将被跳过的任务",
		"current round's skip must carry the (skipped) marker so the frontend can highlight it")
	require.NotContains(t, delta, "(skipped) 上一轮就被跳过的任务",
		"historic skipped TODOs must NOT be re-highlighted on every render")

	// preview must not mutate underlying state
	stats := store.Stats()
	require.Equal(t, 1, stats.Skipped, "store should retain only the previously-skipped TODO")
	require.Equal(t, 1, stats.Pending, "to_skip_now should still be pending in the live store because RenderMarkdownDelta is read-only")
}

func TestSessionPromptState_ApplyVerificationTodoOps_AccumulatesAndRenders(t *testing.T) {
	state := NewSessionPromptState()
	state.ApplyVerificationTodoOps(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "step1", Content: "第一步"},
	})
	state.ApplyVerificationTodoOps(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "step2", Content: "第二步"},
		{Op: "doing", ID: "step1"},
	})

	rendered := state.GetVerificationTodoRendered(VerificationTodoScope{})
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
	require.Equal(t, "", state.GetVerificationTodoRendered(VerificationTodoScope{}),
		"empty state should render empty string so the prompt template can skip the block")
}

func TestVerificationTodoStore_RenderWithCurrentScope_SplitsCurrentAndOther(t *testing.T) {
	store := NewVerificationTodoStore()
	scopeOne := VerificationTodoScope{TaskID: "task-1", TaskIndex: "1-1"}
	scopeTwo := VerificationTodoScope{TaskID: "task-2", TaskIndex: "1-2"}

	store.Apply(scopeOne, false, []VerifyNextMovement{
		{Op: "add", ID: "todo_1_1", Content: "子任务 1-1 的待办"},
		{Op: "done", ID: "todo_1_1"},
	})
	store.Apply(scopeTwo, false, []VerifyNextMovement{
		{Op: "add", ID: "todo_1_2", Content: "子任务 1-2 的待办"},
		{Op: "doing", ID: "todo_1_2"},
	})

	rendered := store.RenderWithCurrentScope(scopeTwo)
	require.Contains(t, rendered, "### CURRENT TASK [task_index=1-2, task_id=task-2]")
	require.Contains(t, rendered, "- [DOING]: [id: todo_1_2]: 子任务 1-2 的待办")
	require.Contains(t, rendered, "### OTHER TASKS (read-only context)")
	require.Contains(t, rendered, "#### task_index=1-1, task_id=task-1")
	require.Contains(t, rendered, "- [x]: [id: todo_1_1]: 子任务 1-1 的待办")
	require.NotContains(t, rendered, "- [DOING]: [id: todo_1_1]")
}

func TestSessionPromptState_GetVerificationTodoMarkdownDelta_IsPreview(t *testing.T) {
	state := NewSessionPromptState()
	state.ApplyVerificationTodoOps(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "step1", Content: "第一步"},
	})

	delta := state.GetVerificationTodoMarkdownDelta(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "done", ID: "step1"},
	})
	require.Contains(t, delta, "(done)")

	// preview must not have mutated underlying state
	stats := state.GetVerificationTodoStats()
	require.Equal(t, 1, stats.Pending)
	require.Zero(t, stats.Done)
}

func TestVerificationTodoStore_ScopeAllowsSameIDAcrossTasks(t *testing.T) {
	taskOne := VerificationTodoScope{TaskID: "task-1", TaskIndex: "1-1"}
	taskTwo := VerificationTodoScope{TaskID: "task-2", TaskIndex: "1-2"}

	store := NewVerificationTodoStore()
	store.Apply(taskOne, false, []VerifyNextMovement{
		{Op: "add", ID: "collect_logs", Content: "任务一收集日志"},
	})
	store.Apply(taskTwo, false, []VerifyNextMovement{
		{Op: "add", ID: "collect_logs", Content: "任务二收集日志"},
		{Op: "doing", ID: "collect_logs"},
	})

	allItems := store.SnapshotItems()
	require.Len(t, allItems, 2)
	require.Equal(t, 1, store.StatsByScope(taskOne).Pending)
	require.Equal(t, 1, store.StatsByScope(taskTwo).Doing)

	taskOneItems := store.SnapshotItemsByScope(taskOne)
	require.Len(t, taskOneItems, 1)
	require.Equal(t, "任务一收集日志", taskOneItems[0].Content)
	require.Equal(t, VerificationTodoStatusPending, taskOneItems[0].Status)

	taskTwoItems := store.SnapshotItemsByScope(taskTwo)
	require.Len(t, taskTwoItems, 1)
	require.Equal(t, "任务二收集日志", taskTwoItems[0].Content)
	require.Equal(t, VerificationTodoStatusDoing, taskTwoItems[0].Status)
}

func TestVerificationTodoStore_ActiveTodoItemsByScope_IgnoresSiblingAndLegacy(t *testing.T) {
	currentScope := VerificationTodoScope{TaskID: "task-current", TaskIndex: "2-1"}
	siblingScope := VerificationTodoScope{TaskID: "task-sibling", TaskIndex: "2-2"}

	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "legacy_todo", Content: "历史遗留 TODO"},
	})
	store.Apply(currentScope, false, []VerifyNextMovement{
		{Op: "add", ID: "current_todo", Content: "当前任务 TODO"},
		{Op: "doing", ID: "current_todo"},
	})
	store.Apply(siblingScope, false, []VerifyNextMovement{
		{Op: "add", ID: "sibling_todo", Content: "兄弟任务 TODO"},
	})

	active := store.ActiveTodoItemsByScope(currentScope)
	require.Len(t, active, 1)
	require.Equal(t, "current_todo", active[0].ID)
	require.Equal(t, VerificationTodoStatusDoing, active[0].Status)

	stats := store.StatsByScope(currentScope)
	require.Zero(t, stats.Pending)
	require.Equal(t, 1, stats.Doing)
	require.True(t, store.HasActiveTodosByScope(currentScope))
}

func TestVerificationTodoStore_MutationCanClaimLegacyItemIntoScope(t *testing.T) {
	scope := VerificationTodoScope{TaskID: "task-claim", TaskIndex: "3-1"}

	store := NewVerificationTodoStore()
	store.Apply(VerificationTodoScope{}, false, []VerifyNextMovement{
		{Op: "add", ID: "legacy_todo", Content: "历史 TODO"},
	})

	require.False(t, store.HasActiveTodosByScope(scope),
		"legacy unscoped items should not block a new scoped task before it explicitly touches them")

	store.Apply(scope, false, []VerifyNextMovement{
		{Op: "done", ID: "legacy_todo"},
	})

	scopedItems := store.SnapshotItemsByScope(scope)
	require.Len(t, scopedItems, 1)
	require.Equal(t, VerificationTodoStatusDone, scopedItems[0].Status)
	require.Equal(t, scope.TaskID, scopedItems[0].ScopeTaskID)
	require.Equal(t, scope.TaskIndex, scopedItems[0].ScopeTaskIndex)
}
