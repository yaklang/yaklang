package aicommon

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// makeBudgetTestTool 造一个工具, 描述长度可控.
// 关键词: SelectToolsByTokenBudget test helper
func makeBudgetTestTool(name, descPad string) *aitool.Tool {
	return aitool.NewWithoutCallback(name, aitool.WithDescription(descPad))
}

// TestSelectToolsByTokenBudget_EmptyAndNil 验证空输入与 nil 元素不 panic.
//
// 关键词: SelectToolsByTokenBudget, empty input, nil tool 容错
func TestSelectToolsByTokenBudget_EmptyAndNil(t *testing.T) {
	require.Len(t, SelectToolsByTokenBudget(nil, 1000, 5), 0)
	require.Len(t, SelectToolsByTokenBudget([]*aitool.Tool{}, 1000, 5), 0)

	// minCount > len 时不应越界, 截断到 len.
	one := []*aitool.Tool{makeBudgetTestTool("a", "desc")}
	require.Len(t, SelectToolsByTokenBudget(one, 1000, 10), 1)
}

// TestSelectToolsByTokenBudget_BelowMin 验证候选数 <= minCount 时直接全量返回.
//
// 关键词: SelectToolsByTokenBudget, 候选不足 minCount, 全量返回
func TestSelectToolsByTokenBudget_BelowMin(t *testing.T) {
	tools := []*aitool.Tool{
		makeBudgetTestTool("t1", "d1"),
		makeBudgetTestTool("t2", "d2"),
		makeBudgetTestTool("t3", "d3"),
	}
	got := SelectToolsByTokenBudget(tools, 100, 20)
	require.Len(t, got, 3)
	require.Equal(t, "t1", got[0].Name)
	require.Equal(t, "t3", got[2].Name)
}

// TestSelectToolsByTokenBudget_MinFloor 验证当 budget 极小但 minCount=20 时,
// 仍要展示 minCount 个工具 (保底).
//
// 关键词: SelectToolsByTokenBudget, budget 1 token 极限, 保底 20
func TestSelectToolsByTokenBudget_MinFloor(t *testing.T) {
	tools := make([]*aitool.Tool, 30)
	for i := 0; i < 30; i++ {
		tools[i] = makeBudgetTestTool(
			fmt.Sprintf("tool_%d", i),
			strings.Repeat("x", 200),
		)
	}
	got := SelectToolsByTokenBudget(tools, 1, 20)
	require.Len(t, got, 20)
	require.Equal(t, "tool_0", got[0].Name)
	require.Equal(t, "tool_19", got[19].Name)
}

// TestSelectToolsByTokenBudget_BudgetCutoffAboveMin 验证 budget 足够覆盖比
// minCount 更多但少于全部的情况, 截断点严格按 token 计算.
//
// 关键词: SelectToolsByTokenBudget, budget 截断, 超过 minCount 继续累加
func TestSelectToolsByTokenBudget_BudgetCutoffAboveMin(t *testing.T) {
	// 每个工具描述短小, 单行 token 数稳定; minCount=20.
	tools := make([]*aitool.Tool, 100)
	for i := 0; i < 100; i++ {
		tools[i] = makeBudgetTestTool(
			fmt.Sprintf("tool_%d", i),
			"short description",
		)
	}
	// 先量出单行 token 数, 再给一个能覆盖 ~30 行的预算.
	sampleLine := fmt.Sprintf("* `%s`: %s\n", "tool_0", "short description")
	perLine := ytoken.CalcTokenCount(sampleLine)
	require.Greater(t, perLine, 0)
	budget := perLine * 30

	got := SelectToolsByTokenBudget(tools, budget, 20)
	require.GreaterOrEqual(t, len(got), 20)
	require.LessOrEqual(t, len(got), 100)
	// 截断点不应超过 budget/perLine 太多 (允许 ±2 偏差, 因为不同 tool 描述会有
	// 微小差异; 实测应在 28~32 之间).
	require.InDelta(t, 30, len(got), 3)
}

// TestSelectToolsByTokenBudget_DefaultBudget 验证主路径用的默认预算 (3000 token,
// 保底 20) 在典型场景下能选出至少 20 个、不超过候选池上限的工具.
//
// 关键词: SelectToolsByTokenBudget, 主路径默认预算, 主回归保护
func TestSelectToolsByTokenBudget_DefaultBudget(t *testing.T) {
	// 95 个工具, 模拟当前总工具量.
	tools := make([]*aitool.Tool, 95)
	for i := 0; i < 95; i++ {
		tools[i] = makeBudgetTestTool(
			fmt.Sprintf("tool_%d", i),
			"a moderate length description that talks about what the tool does in practice",
		)
	}
	got := SelectToolsByTokenBudget(tools, ToolInventoryTokenBudget, ToolInventoryMinCount)
	require.GreaterOrEqual(t, len(got), ToolInventoryMinCount,
		"default 3K budget should always show at least %d tools", ToolInventoryMinCount)
	require.LessOrEqual(t, len(got), 95,
		"never exceed the prioritized candidate pool size")
}

// TestSelectToolsByTokenBudget_NilToolsSkipped 验证 prioritized 切片里的 nil
// 元素不计入 token 也不被丢弃位置 (按 index 推进).
//
// 关键词: SelectToolsByTokenBudget, nil tool 跳过, index 推进不丢位
func TestSelectToolsByTokenBudget_NilToolsSkipped(t *testing.T) {
	tools := []*aitool.Tool{
		nil,
		makeBudgetTestTool("real_a", "desc"),
		nil,
		makeBudgetTestTool("real_b", "desc"),
	}
	got := SelectToolsByTokenBudget(tools, 10_000, 1)
	require.Len(t, got, 4)
}
