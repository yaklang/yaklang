package reactloops

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithDisableTodoSnapshot(t *testing.T) {
	t.Run("default renders todo snapshot", func(t *testing.T) {
		loop := &ReActLoop{}
		require.True(t, loop.shouldRenderTodoSnapshot())
	})

	t.Run("opt-out skips todo snapshot", func(t *testing.T) {
		loop := &ReActLoop{}
		WithDisableTodoSnapshot(true)(loop)
		require.False(t, loop.shouldRenderTodoSnapshot())
	})

	t.Run("explicit false keeps todo snapshot enabled", func(t *testing.T) {
		loop := &ReActLoop{}
		WithDisableTodoSnapshot(false)(loop)
		require.True(t, loop.shouldRenderTodoSnapshot())
	})
}

func TestTodoListTemplateEnforcesFrontierCurrentDepthFirstPolicy(t *testing.T) {
	require.Contains(t, todoListTemplate, "只由正常 ReAct 动作携带的 todo_delta 累计维护")
	require.Contains(t, todoListTemplate, "TODO LIST 是只读投影")
	require.Contains(t, todoListTemplate, "新增、细化、关闭、延期、恢复或切换必须在同一正常动作中携带 todo_delta")
	require.Contains(t, todoListTemplate, "开放项共同构成待返回的 Frontier")
	require.Contains(t, todoListTemplate, "当前主要矛盾")
	require.Contains(t, todoListTemplate, "先用 todo_delta 记录分叉，再执行 Current")
	require.Contains(t, todoListTemplate, "add / update 全部合格分支")
	require.Contains(t, todoListTemplate, "具体目标、触发证据、可证伪假设和恢复后的第一步")
	require.Contains(t, todoListTemplate, "工具本轮未暴露不是拒绝入队的理由")
	require.Contains(t, todoListTemplate, "同级有效路径先保存在 Frontier")
	require.Contains(t, todoListTemplate, "严格优先级逆转")
	require.Contains(t, todoListTemplate, "同一 todo_delta 中 close 旧项并设置下一 current")
}
