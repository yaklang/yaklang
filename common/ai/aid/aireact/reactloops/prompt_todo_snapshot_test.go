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
	require.Contains(t, todoListTemplate, "具体入口就是合格覆盖分支")
	require.Contains(t, todoListTemplate, "不要求先有漏洞信号")
	require.Contains(t, todoListTemplate, "验证型分支再写可证伪假设")
	require.Contains(t, todoListTemplate, "工具本轮未暴露不是拒绝入队的理由")
	require.Contains(t, todoListTemplate, "深度优先不等于重复同一种失败请求")
	require.Contains(t, todoListTemplate, "单次工具/参数/连接/认证失败")
	require.Contains(t, todoListTemplate, "不能 close、deferred 或 finish")
	require.Contains(t, todoListTemplate, "改变方法、编码、参数通道、请求形态、会话、基线或观察通道")
	require.Contains(t, todoListTemplate, "在同一 todo_delta 中设置下一 current")
	require.NotContains(t, todoListTemplate, "严格优先级逆转")
}
